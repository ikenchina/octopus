package executor

import (
	"context"
	"time"

	"github.com/ikenchina/octopus/common/errorutil"
	"github.com/ikenchina/octopus/common/metrics"
	"github.com/ikenchina/octopus/common/operator"
	"github.com/ikenchina/octopus/common/slice"
	"github.com/ikenchina/octopus/define"
	"github.com/ikenchina/octopus/tc/app/model"
)

var (
	sagaBranchTimer = metrics.NewTimer("dtx", "saga_branch", "saga branch timer", []string{"branch"})
	sagaGauge       = metrics.NewGaugeVec("dtx", "saga_txn", "in flight sagas", []string{"state"})
)

// SagaExecutor
func NewSagaExecutor(cfg Config) (*SagaExecutor, error) {
	se := &SagaExecutor{}
	se.baseExecutor = &baseExecutor{
		cfg:     cfg,
		process: se.process,
	}
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
	go se.startCleanup(define.TxnTypeSaga)
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

	err = se.cfg.Store.Save(st.Ctx, st.Txn)
	if err != nil {
		se.finishTask(st, err)
		return st.future()
	}

	_ = se.queueTask(st)
	return st.future()
}

func (se *SagaExecutor) startCrontab() {
	defer errorutil.Recovery()

	limit := 10
	se.cronjob(func() time.Duration {
		duration := se.cronDuration
		tasks, err := se.cfg.Store.FindLeaseExpired(context.TODO(), define.TxnTypeSaga, nil, limit)
		if err != nil {
			return duration
		}

		for _, task := range tasks {
			t := se.newTask(context.Background(), task)
			if se.startTask(t) != nil {
				continue
			}
			_ = se.queueTask(t)
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
	if task.State == define.TxnStatePrepared {
		se.processTask(task)
		return
	}

	//
	srcStates := []string{}
	dstState := define.TxnStateCommitted
	if slice.InSlice(task.State, define.TxnStateCommitting, define.TxnStateCommitted) {
		// committing|committed -> committed
		srcStates = []string{define.TxnStateCommitted, define.TxnStateCommitting}
		if task.State == define.TxnStateCommitting {
			if se.notify(task) != nil {
				return
			}
		}
	} else if slice.InSlice(task.State, define.TxnStateFailed, define.TxnStateAborted) {
		// failed|aborted -> aborted
		srcStates = []string{define.TxnStateAborted, define.TxnStateFailed}
		if task.State == define.TxnStateFailed {
			if se.notify(task) != nil {
				return
			}
		}
		dstState = define.TxnStateAborted
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
		if branch.BranchType == define.BranchTypeCommit {
			if branch.State == define.TxnStateAborted ||
				((branch.State == define.TxnStateFailed) &&
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
		if branch.BranchType != define.BranchTypeCompensation ||
			branch.State == define.TxnStateCommitted {
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
	err := se.saveState(task, []string{define.TxnStatePrepared, define.TxnStateFailed},
		define.TxnStateFailed)
	if err != nil {
		return
	}
	if se.notify(task) != nil {
		return
	}

	// failed -> aborted
	err = se.saveState(task, []string{define.TxnStateFailed, define.TxnStateAborted},
		define.TxnStateAborted)
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
	task.SetLessee(se.cfg.Lessee)
	task.SetLeaseExpireTime(time.Now().Add(duration))
	err := se.cfg.Store.GrantLeaseIncBranch(task.Ctx, task.Txn, branch, duration)
	if err != nil {
		return err
	}
	branch.TryCount += 1
	return nil
}

func (se *SagaExecutor) processPrepared(task *actionTask) {
	for _, branch := range task.Branches {
		if branch.BranchType != define.BranchTypeCommit ||
			branch.State == define.TxnStateCommitted {
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
	err := se.saveState(task, []string{define.TxnStatePrepared, define.TxnStateCommitting},
		define.TxnStateCommitting)
	if err != nil {
		return
	}

	err = se.notify(task)
	if err != nil {
		return
	}

	// committing -> committed
	err = se.saveState(task, []string{define.TxnStateCommitting, define.TxnStateCommitted},
		define.TxnStateCommitted)
	if err != nil {
		return
	}
	se.finishTask(task, nil)
}

func (se *SagaExecutor) commitBranch(task *actionTask, branch *model.Branch) error {
	resp, err := se.processBranch(task, branch)
	if err == nil {
		branch.SetResponse(resp)
		branch.SetState(define.TxnStateCommitted)
		return nil
	}

	branch.SetState(define.TxnStateFailed)
	err2 := se.cfg.Store.UpdateBranchConditions(task.Ctx, branch,
		func(oldTxn *model.Txn, oldBranch *model.Branch) error {
			if !slice.Contain([]string{define.TxnStatePrepared, define.TxnStateFailed}, oldBranch.State) {
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
		branch.SetState(define.TxnStateCommitted)
	} else {
		se.schedule(task, branch.RetryDuration())
	}
	return err
}

func (se *SagaExecutor) processBranch(task *actionTask, branch *model.Branch) (resp string, err error) {
	ctx, cancel := context.WithTimeout(task.Ctx, branch.Timeout)
	defer cancel()
	defer sagaBranchTimer.Timer()(branch.BranchType)

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
	defer sagaBranchTimer.Timer()("notify")

	ctx, cancel := context.WithTimeout(task.Ctx, task.Txn.NotifyTimeout)
	defer cancel()

	task.SetState(operator.IfElse(task.State == define.TxnStateCommitting,
		define.TxnStateCommitted, define.TxnStateAborted).(string))

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
		task.SetState(operator.IfElse(task.State == define.TxnStateCommitted,
			define.TxnStateCommitting, define.TxnStateFailed).(string))
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
