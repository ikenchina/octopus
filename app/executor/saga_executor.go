package executor

import (
	"context"
	"time"

	"github.com/ikenchina/octopus/app/model"
	"github.com/ikenchina/octopus/common/errorutil"
	"github.com/ikenchina/octopus/common/metrics"
	"github.com/ikenchina/octopus/common/operator"
	"github.com/ikenchina/octopus/common/slice"
)

var (
	sagaExecTimer = metrics.NewTimer("dtx", "saga_txn", "saga timer", []string{"branch"})
	sagaGauge     = metrics.NewGaugeVec("dtx", "saga_txn", "in flight sagas", []string{"state"})
)

// SagaExecutor
func NewSagaExecutor(store model.ModelStorage, maxConcurrency int, lessee string) (*SagaExecutor, error) {
	se := &SagaExecutor{}
	se.baseExecutor = newExecutor(se.process, store, maxConcurrency, lessee)
	return se, nil
}

type SagaExecutor struct {
	*baseExecutor
}

func (se *SagaExecutor) Start() error {
	err := se.baseExecutor.start()
	if err != nil {
		return err
	}

	go se.startCrontab()
	return nil
}

func (se *SagaExecutor) Stop() error {
	return se.baseExecutor.stop()
}

func (se *SagaExecutor) Get(ctx context.Context, gtid string) (*model.Txn, error) {
	return se.getTask(ctx, gtid)
}

func (se *SagaExecutor) newTask(ctx context.Context, txn *model.Txn) *actionTask {
	t := newTask(ctx, txn, func(s, e time.Time, err error) {
		sagaGauge.Set(float64(se.taskCount()), "inflight")
	})
	return t
}

func (se *SagaExecutor) Commit(ctx context.Context, txn *model.Txn) *ActionFuture {
	se.wait.Add(1)
	defer se.wait.Done()

	st := se.newTask(ctx, txn)
	if se.isClosed() {
		st.finish(ErrExecutorClosed)
		return st.future()
	}

	err := se.startTask(st)
	if err != nil {
		st.finish(err)
		return st.future()
	}

	err = se.store.Save(st.Ctx, st.Txn)
	if err != nil {
		se.finishTask(st, err)
		return st.future()
	}

	se.queueTask(st)
	return st.future()
}

func (se *SagaExecutor) startCrontab() {
	defer errorutil.Recovery()

	limit := 10
	se.cronjob(func() time.Duration {
		duration := se.cronDuration
		tasks, err := se.store.FindLeaseExpired(context.TODO(), model.TxnTypeSaga, nil, limit)
		if err != nil {
			return duration
		}

		for _, task := range tasks {
			t := se.newTask(context.Background(), task)
			if se.startTask(t) != nil {
				continue
			}
			se.queueTask(t)
		}
		if len(tasks) == limit {
			duration = time.Millisecond * 5
		}
		return duration
	})
}

//  finite state machine of task
func (se *SagaExecutor) process(task *actionTask) {
	se.wait.Add(1)
	defer se.wait.Done()

	select {
	case <-task.Ctx.Done():
		se.finishTask(task, ErrTimeout)
		return
	case <-se.closeChan:
		se.finishTask(task, ErrExecutorClosed)
		return
	default:
	}

	sagaGauge.Set(float64(len(se.taskChan)), "inflight")

	// prepare
	if task.State == model.TxnStatePrepared {
		se.processTask(task)
		return
	}

	//
	srcStates := []string{}
	dstState := model.TxnStateCommitted
	if slice.InSlice(task.State, model.TxnStateCommitting, model.TxnStateCommitted) {
		// committing|committed -> committed
		srcStates = []string{model.TxnStateCommitted, model.TxnStateCommitting}
		if task.State == model.TxnStateCommitting {
			if se.notify(task) != nil {
				return
			}
		}
	} else if slice.InSlice(task.State, model.TxnStateFailed, model.TxnStateAborted) {
		// failed|aborted -> aborted
		srcStates = []string{model.TxnStateAborted, model.TxnStateFailed}
		if task.State == model.TxnStateFailed {
			if se.notify(task) != nil {
				return
			}
		}
		dstState = model.TxnStateAborted
	} else {
		se.finishTask(task, ErrInvalidState)
	}

	err := se.saveState(task, srcStates, dstState)
	if err != nil {
		return
	}
	se.finishTask(task, nil)
}

func (se *SagaExecutor) processTask(task *actionTask) {
	if se.shouldRollback(task) {
		se.processRollback(task)
	} else {
		se.processPrepared(task)
	}
}

func (se *SagaExecutor) shouldRollback(task *actionTask) bool {
	if task.ExpireTime.Before(time.Now()) { // notice : clock skew between database and server
		return true
	}

	for _, branch := range task.Branches {
		if branch.BranchType == model.BranchTypeCommit {
			if branch.State == model.TxnStateAborted ||
				((branch.State == model.TxnStateFailed) &&
					branch.TryCount >= (branch.Retry.MaxRetry+1)) {
				return true
			}
		}
	}
	return false
}

func (se *SagaExecutor) processRollback(task *actionTask) {
	// 回滚所有分支，有的分支可能没有执行
	// 但是也必须进行回滚，原因：
	// 1. 可能已经执行commit，但TC认为没有执行
	// 2. 可能还没有执行commit，但回滚分支后，commit请求达到RM
	// 这种情况RM应该将rollback信息存储下来，以避免随后的commit请求被执行
	// 根本原因还是RM端没有进行prepare，也没有branch的commit超时时间导致的

	for i := len(task.Branches) - 1; i >= 0; i-- {
		branch := task.Branches[i]
		if branch.BranchType != model.BranchTypeCompensation ||
			branch.State == model.TxnStateCommitted {
			continue
		}

		err := se.grantLease(task, branch)
		if err != nil {
			se.finishTask(task, err)
			return
		}

		err = se.rollbackBranch(task, branch)
		if err != nil {
			return
		}
	}

	// prepared -> failed
	err := se.saveState(task, []string{model.TxnStatePrepared, model.TxnStateFailed},
		model.TxnStateFailed)
	if err != nil {
		return
	}
	if se.notify(task) != nil {
		return
	}

	// failed -> aborted
	err = se.saveState(task, []string{model.TxnStateFailed, model.TxnStateAborted},
		model.TxnStateAborted)
	if err != nil {
		return
	}

	se.finishTask(task, nil)
}

func (se *SagaExecutor) grantLease(task *actionTask, branch *model.Branch) error {
	duration := branch.Timeout
	if branch.Retry.Constant != nil {
		duration += branch.Retry.Constant.Duration
	}
	task.SetLessee(se.Lessee)
	task.SetLeaseExpireTime(time.Now().Add(duration))
	err := se.store.GrantLeaseIncBranch(task.Ctx, task.Txn, branch, duration)
	if err != nil {
		return err
	}
	branch.TryCount += 1
	return nil
}

func (se *SagaExecutor) processPrepared(task *actionTask) {
	for _, branch := range task.Branches {
		if branch.BranchType != model.BranchTypeCommit ||
			branch.State == model.TxnStateCommitted {
			continue
		}

		if task.ExpireTime.Before(time.Now()) {
			se.schedule(task, time.Millisecond*100)
			return
		}
		err := se.grantLease(task, branch)
		if err != nil {
			se.finishTask(task, err)
			return
		}
		err = se.commitBranch(task, branch)
		if err != nil {
			return
		}
	}

	// prepare -> committing
	err := se.saveState(task, []string{model.TxnStatePrepared, model.TxnStateCommitting},
		model.TxnStateCommitting)
	if err != nil {
		return
	}

	err = se.notify(task)
	if err != nil {
		return
	}

	// committing -> committed
	err = se.saveState(task, []string{model.TxnStateCommitting, model.TxnStateCommitted},
		model.TxnStateCommitted)
	if err != nil {
		return
	}
	se.finishTask(task, nil)
}

func (se *SagaExecutor) commitBranch(task *actionTask, branch *model.Branch) error {
	resp, err := se.processBranch(task, branch)
	if err == nil {
		branch.SetResponse(resp)
		branch.SetState(model.TxnStateCommitted)
		return nil
	}

	branch.SetState(model.TxnStateFailed)
	err2 := se.store.UpdateBranchConditions(task.Ctx, branch,
		func(oldTxn *model.Txn, oldBranch *model.Branch) error {
			if !slice.Contain([]string{model.TxnStatePrepared, model.TxnStateFailed}, oldBranch.State) {
				return ErrInvalidState
			}
			return nil
		})
	if se.finishTaskFatalError(task, err2) { // ignore other errors
		return err2
	}

	if branch.CanTry() {
		se.schedule(task, se.branchRetry(branch))
	} else {
		se.schedule(task, time.Millisecond*100)
	}
	return err
}

func (se *SagaExecutor) rollbackBranch(task *actionTask, branch *model.Branch) error {
	resp, err := se.processBranch(task, branch)
	if err == nil {
		branch.SetResponse(resp)
		branch.SetState(model.TxnStateCommitted)
	} else {
		se.schedule(task, branch.RetryDuration())
	}
	return err
}

func (se *SagaExecutor) processBranch(task *actionTask, branch *model.Branch) (resp string, err error) {
	ctx, cancel := context.WithTimeout(task.Ctx, branch.Timeout)
	defer cancel()
	defer sagaExecTimer.Timer()(branch.BranchType)

	nf := NewActionNotify(ctx, task.Txn, branch.BranchType, branch.Action, branch.Payload)
	se.notifyChan <- nf

	select {
	case err = <-nf.DoneChan:
		resp = nf.Msg
	case <-task.Ctx.Done():
		err = ErrTimeout
	case <-se.closeChan:
		err = ErrExecutorClosed
	}
	return
}

func (se *SagaExecutor) notify(task *actionTask) (err error) {
	if !task.NeedNotify() {
		return nil
	}
	defer sagaExecTimer.Timer()("notify")

	ctx, cancel := context.WithTimeout(task.Ctx, task.Txn.NotifyTimeout)
	defer cancel()

	task.SetState(operator.IfElse(task.State == model.TxnStateCommitting,
		model.TxnStateCommitted, model.TxnStateAborted).(string))

	sn := NewActionNotify(ctx, task.Txn, "", task.NotifyAction, "")
	se.notifyChan <- sn

	select {
	case err = <-sn.DoneChan:
		task.IncrNotify()
	case <-task.Ctx.Done():
		err = ErrTimeout
	case <-se.closeChan:
		err = ErrExecutorClosed
	}
	if err != nil {
		task.SetState(operator.IfElse(task.State == model.TxnStateCommitted,
			model.TxnStateCommitting, model.TxnStateFailed).(string))
		se.schedule(task, task.NotifyRetry)
	}
	return
}

func (se *SagaExecutor) branchRetry(branch *model.Branch) time.Duration {
	if branch.Retry.Constant != nil {
		return branch.Retry.Constant.Duration
	}
	return time.Millisecond * 100
}
