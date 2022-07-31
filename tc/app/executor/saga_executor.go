package executor

import (
	"context"
	"time"

	"github.com/ikenchina/octopus/common/errorutil"
	logutil "github.com/ikenchina/octopus/common/log"
	"github.com/ikenchina/octopus/common/operator"
	"github.com/ikenchina/octopus/common/slice"
	"github.com/ikenchina/octopus/define"
	"github.com/ikenchina/octopus/tc/app/model"
)

// SagaExecutor
func NewSagaExecutor(cfg Config) (*SagaExecutor, error) {
	se := &SagaExecutor{}
	se.baseExecutor = &baseExecutor{
		cfg:     cfg,
		process: se.process,
		txnType: define.TxnTypeSaga,
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
		stateGauge.Set(float64(se.taskCount()), se.txnType, "inflight")
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
	if err != nil { // already exist
		st.finish(err)
		return st.future()
	}

	// state is committing
	se.initLease(st)
	st.SetState(define.TxnStateCommitting)
	err = se.cfg.Store.Save(st.Ctx, st.Txn)
	if err != nil {
		se.finishTask(st, err)
		return st.future()
	}

	err = se.queueTask(st)
	if err != nil {
		se.finishTask(st, err)
	}
	return st.future()
}

func (se *SagaExecutor) startCrontab() {
	defer errorutil.Recovery()

	limit := se.cfg.LeaseExpiredLimit
	if limit <= 0 {
		limit = 20
	}

	se.cronjob(func() time.Duration {
		ctx, cancel := context.WithTimeout(context.TODO(), time.Second*10)
		defer cancel()
		duration := se.cronDuration
		// lease expired,
		tasks, err := se.cfg.Store.FindRunningLeaseExpired(ctx, define.TxnTypeSaga, limit)
		if err != nil {
			return duration
		}

		logutil.Logger(ctx).Sugar().Debugf("cronjob : %d", len(tasks))
		queuedTask := 0
		for _, task := range tasks {
			t := se.newTask(context.Background(), task)
			if se.startTask(t) != nil {
				continue
			}
			queuedTask++
			err = se.queueTask(t)
			if err != nil {
				se.finishTask(t, err)
			}
		}
		if queuedTask == limit {
			duration = time.Millisecond * 5
		}
		return duration
	})
}

//  finite state machine of task
/*
 *    committing -> precommitted -> committed
 *    rolling -> preaborted -> aborted
 */
func (se *SagaExecutor) process(task *actionTask) {
	se.wait.Add(1)
	defer se.wait.Done()

	logutil.Logger(task.Ctx).Sugar().Debugf("process : %s, %s", task.Gtid, task.State)

	select {
	case <-task.Ctx.Done():
		task.notify(ErrCancel)
	case <-se.closeChan:
		se.finishTask(task, ErrExecutorClosed)
		return
	default:
	}

	stateGauge.Set(float64(len(se.taskChan)), se.txnType, "inflight")

	// committing, rolling
	if slice.InSlice(task.State, define.TxnStateCommitting, define.TxnStateRolling) {
		se.processTask(task)
		return
	}

	//
	srcStates := []string{}
	dstState := define.TxnStateCommitted
	if slice.InSlice(task.State, define.TxnStatePreCommitted, define.TxnStateCommitted) {
		// precommitted|committed -> committed
		srcStates = []string{define.TxnStateCommitted, define.TxnStatePreCommitted}
		if task.State == define.TxnStatePreCommitted {
			if se.notify(task) != nil {
				return
			}
		}
	} else if slice.InSlice(task.State, define.TxnStatePreAborted, define.TxnStateAborted) {
		// preaborted|aborted -> aborted
		srcStates = []string{define.TxnStateAborted, define.TxnStatePreAborted}
		if task.State == define.TxnStatePreAborted {
			if se.notify(task) != nil {
				return
			}
		}
		dstState = define.TxnStateAborted
	} else {
		se.finishTask(task, ErrInvalidState)
		logutil.Logger(task.Ctx).Sugar().Errorf("invalid state : %s, %s", task.Gtid, task.State)
		return
	}

	err := se.updateState(task, srcStates, dstState)
	if err != nil {
		return
	}
	se.finishTask(task, nil)
}

func (se *SagaExecutor) processTask(task *actionTask) {
	if se.shouldRollback(task) {
		se.processRollback(task)
	} else {
		se.processCommitting(task)
	}
}

func (se *SagaExecutor) shouldRollback(task *actionTask) bool {
	if task.State == define.TxnStateRolling {
		return true
	}
	allCommitted := true
	for _, branch := range task.Branches {
		if branch.BranchType == define.BranchTypeCommit {
			if branch.State == define.TxnStateRolling || !branch.CanTry() {
				return true
			}
			if branch.State != define.TxnStateCommitted {
				allCommitted = false
			}
		} else {
			if branch.State == define.TxnStateCommitted {
				return true
			}
		}
	}

	if allCommitted {
		return false
	}

	if task.ExpireTime.Before(time.Now()) { // notice : clock skew between database and server
		return true
	}

	return false
}

func (se *SagaExecutor) processRollback(task *actionTask) {
	defer txnTimer.Timer()(se.txnType, "rollback")

	logutil.Logger(task.Ctx).Sugar().Debugf("processRollback : %s, %s", task.Gtid, task.State)

	// 回滚所有分支，有的分支可能没有执行
	// 但是也必须进行回滚，原因：
	// 1. 可能已经执行commit，但TC认为没有执行
	// 2. 可能还没有执行commit，但回滚分支后，commit请求达到RM
	// 这种情况RM应该将rollback信息存储下来，以避免随后的commit请求被执行
	// 根本原因还是RM端没有进行prepare，也没有branch的commit超时时间导致的

	err := se.updateState(task, []string{define.TxnStateCommitting, define.TxnStateRolling}, define.TxnStateRolling)
	if err != nil {
		return
	}

	for i := len(task.Branches) - 1; i >= 0; i-- {
		branch := task.Branches[i]
		if branch.BranchType != define.BranchTypeCompensation ||
			branch.State == define.TxnStateCommitted {
			continue
		}

		err := se.grantLease(task, branch, []string{define.TxnStateRolling})
		if err != nil {
			se.finishTask(task, err)
			return
		}

		err = se.commitBranch(task, branch)
		if err != nil {
			return
		}
	}

	// rolling -> pre_aborted
	if task.NeedNotify() {
		err := se.updateState(task, []string{define.TxnStateRolling, define.TxnStatePreAborted}, define.TxnStatePreAborted)
		if err != nil {
			return
		}
		if se.notify(task) != nil {
			return
		}
	} else {
		task.SetState(define.TxnStatePreAborted)
	}

	// pre_aborted -> aborted
	err = se.updateState(task, []string{define.TxnStateRolling, define.TxnStatePreAborted, define.TxnStateAborted},
		define.TxnStateAborted)
	if err != nil {
		logutil.Logger(task.Ctx).Sugar().Errorf("processRollback save state : %s, %s", task.Gtid, task.State)
		return
	}

	se.finishTask(task, nil)
}

func (se *SagaExecutor) initLease(task *actionTask) {
	duration := time.Second * 2
	if len(task.Branches) > 0 {
		if duration > task.Branches[0].Timeout {
			duration = task.Branches[0].Timeout
		}
	}
	task.LeaseExpireTime = time.Now().Add(duration).Add(se.cfg.Store.Timeout())
	if task.LeaseExpireTime.After(task.ExpireTime) {
		task.SetLeaseExpireTime(task.ExpireTime)
	}
}

func (se *SagaExecutor) grantLease(task *actionTask, branch *model.Branch, states []string) error {
	duration := branch.Timeout + se.cfg.Store.Timeout()
	if branch.Retry.Constant != nil {
		duration += branch.Retry.Constant.Duration
	}
	task.SetLessee(se.cfg.Lessee)
	task.SetLeaseExpireTime(time.Now().Add(duration))
	err := se.cfg.Store.GrantLeaseIncBranchCheckState(task.Ctx, task.Txn, branch, duration, states)
	if err != nil {
		logutil.Logger(context.Background()).Sugar().Errorf("grantLease : %v,  %v,  %v", task.Gtid, err, states)
		return err
	}
	branch.TryCount += 1
	return nil
}

func (se *SagaExecutor) processCommitting(task *actionTask) {
	defer txnTimer.Timer()(se.txnType, "committing")

	// prepared
	for _, branch := range task.Branches {
		if branch.BranchType != define.BranchTypeCommit ||
			branch.State == define.TxnStateCommitted {
			continue
		}

		if task.ExpireTime.Before(time.Now()) {
			se.schedule(task, time.Millisecond*100)
			return
		}

		err := se.grantLease(task, branch, []string{define.TxnStateCommitting})
		if err != nil {
			logutil.Logger(task.Ctx).Sugar().Errorf("grant lease error : %s, %v, %v", task.Gtid, branch.Bid, err)
			se.finishTask(task, err)
			return
		}
		err = se.commitBranch(task, branch)
		if err != nil {
			return
		}
	}

	// committing --> precommitted
	if task.NeedNotify() {
		err := se.updateState(task, []string{define.TxnStateCommitting, define.TxnStatePreCommitted},
			define.TxnStatePreCommitted)
		if err != nil {
			return
		}

		err = se.notify(task)
		if err != nil {
			return
		}
	} else {
		task.SetState(define.TxnStatePreCommitted)
	}

	// precommitted -> committed
	err := se.updateState(task, []string{define.TxnStateCommitting, define.TxnStatePreCommitted, define.TxnStateCommitted},
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

	logutil.Logger(task.Ctx).Sugar().Errorf("commitBranch : %v,  %v", task.Gtid, err)
	if branch.BranchType == define.BranchTypeCommit {
		if !branch.CanTry() {
			branch.State = define.TxnStateRolling
		}
	}
	// branch.SetState(define.TxnStateRolling)
	// err2 := se.cfg.Store.UpdateBranchConditions(task.Ctx, branch,
	// 	func(oldTxn *model.Txn, oldBranch *model.Branch) error {
	// 		if !slice.Contain([]string{define.committing, define.TxnStatePreAborted}, oldBranch.State) {
	// 			return ErrInvalidState
	// 		}
	// 		return nil
	// 	})
	// if se.finishTaskFatalError(task, err2) { // ignore other errors
	// 	return err2
	// }

	se.schedule(task, se.branchRetry(branch))
	return err
}

func (se *SagaExecutor) processBranch(task *actionTask, branch *model.Branch) (resp []byte, err error) {
	ctx, cancel := context.WithTimeout(task.Ctx, branch.Timeout)
	defer cancel()
	defer branchTimer.Timer()(se.txnType, branch.BranchType)

	payload := branch.Payload
	if len(payload) == 0 && branch.BranchType == define.BranchTypeCompensation {
		for _, bb := range task.Branches {
			if bb.Bid == branch.Bid && bb.BranchType == define.BranchTypeCommit {
				payload = bb.Payload
			}
		}
	}

	nf := NewActionNotify(ctx, task.Txn, branch.BranchType, branch.Bid, branch.Action, payload)
	se.notifyChan <- nf

	select {
	case err = <-nf.DoneChan:
		resp = nf.Msg
	case <-task.Ctx.Done():
		err = ErrTimeout
	case <-se.closeChan:
		err = ErrExecutorClosed
	}

	if err != nil {
		logutil.Logger(context.TODO()).Sugar().Errorf("process branch error : %s %d %v",
			branch.Gtid, branch.Bid, err)
	}
	return
}

func (se *SagaExecutor) notify(task *actionTask) (err error) {
	if !task.NeedNotify() {
		return nil
	}
	defer branchTimer.Timer()(se.txnType, "notify")

	ctx, cancel := context.WithTimeout(task.Ctx, task.Txn.NotifyTimeout)
	defer cancel()

	oState := task.State
	task.SetState(operator.IfElse(task.State == define.TxnStatePreCommitted,
		define.TxnStateCommitted, define.TxnStateAborted).(string))

	sn := NewActionNotify(ctx, task.Txn, "", 0, task.NotifyAction, nil)
	se.notifyChan <- sn

	select {
	case err = <-sn.DoneChan:
		task.IncrNotify()
	case <-task.Ctx.Done():
		err = ErrTimeout
	case <-se.closeChan:
		err = ErrExecutorClosed
	}

	task.SetState(oState)
	if err != nil {
		se.schedule(task, task.NotifyRetry)
		logutil.Logger(task.Ctx).Sugar().Errorf("notify : %s, %v", task.Gtid, err)
	}
	return
}

func (se *SagaExecutor) branchRetry(branch *model.Branch) time.Duration {
	if branch.CanTry() {
		if branch.Retry.Constant != nil {
			return branch.Retry.Constant.Duration
		}
	}
	return time.Millisecond * 100
}
