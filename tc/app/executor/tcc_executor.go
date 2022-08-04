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

//  finite state machine
/*
 *     prepare -> committing  -> committed
 *     prepare -> rolling -> aborted
 */
type TccExecutor struct {
	*baseExecutor
}

func NewTccExecutor(cfg Config) (*TccExecutor, error) {
	te := &TccExecutor{}
	te.baseExecutor = &baseExecutor{
		cfg:     cfg,
		process: te.process,
		txnType: define.TxnTypeTcc,
	}
	return te, nil
}

func (se *TccExecutor) Start() (err error) {
	err = se.baseExecutor.start()
	if err != nil {
		return
	}
	go se.startCrontab()
	go se.startCleanup(define.TxnTypeTcc)
	return
}

func (se *TccExecutor) Stop() error {
	return se.baseExecutor.stop()
}

// find expired tasks :
//    1. if state is prepared : abort it
// find committing and rolling lease expired tasks :
//    1. if state is committing : commit it
//    2. if state is rolling : abort it
func (te *TccExecutor) startCrontab() {
	defer errorutil.Recovery()

	limit := te.cfg.LeaseExpiredLimit
	if limit <= 0 {
		limit = 20
	}
	te.cronjob(func() time.Duration {
		ctx, cancel := context.WithTimeout(context.TODO(), time.Second*10)
		defer cancel()
		duration := te.cronDuration
		// expired and state is prepared
		tasks, err := te.cfg.Store.FindPreparedExpired(ctx, define.TxnTypeTcc, limit)
		if err != nil {
			logutil.Logger(context.Background()).Sugar().Errorf("Find tcc expired transactions error : %v", err)
		}

		// lease expired and state is not committed, aborted, prepared
		tasks2, err := te.cfg.Store.FindRunningLeaseExpired(ctx, define.TxnTypeTcc, limit)
		if err != nil {
			return duration
		}
		tasks = append(tasks, tasks2...)

		queuedTask := 0
		found := make(map[string]struct{})
		for _, task := range tasks {
			if _, ok := found[task.Gtid]; ok {
				continue
			}
			found[task.Gtid] = struct{}{}
			t := te.newTask(context.Background(), task, "cron")
			if te.startTask(t) != nil {
				continue
			}
			queuedTask++
			err = te.queueTask(t)
			if err != nil {
				te.finishTask(t, err)
			}
		}
		if queuedTask == limit {
			duration = time.Millisecond * 5
		}
		return duration
	})
}

func (te *TccExecutor) newTask(ctx context.Context, txn *model.Txn, process string) *actionTask {
	t := newTask(ctx, txn, process, func(s, e time.Time, err error) {
		stateGauge.Set(float64(te.taskCount()), te.txnType, "inflight")
	})
	return t
}

func (te *TccExecutor) Get(ctx context.Context, gtid string) (*model.Txn, error) {
	return te.getTask(ctx, gtid)
}

// state : prepared
func (te *TccExecutor) Prepare(ctx context.Context, txn *model.Txn) *ActionFuture {
	te.wait.Add(1)
	defer te.wait.Done()
	txn.Branches = nil
	st := te.newTask(ctx, txn, "prepare")
	if te.isClosed() {
		st.finish(ErrExecutorClosed)
		return st.future()
	}

	err := te.cfg.Store.Save(ctx, txn)
	st.finish(err)
	return st.future()
}

// state : prepared
func (te *TccExecutor) Register(ctx context.Context, txn *model.Txn) *ActionFuture {
	te.wait.Add(1)
	defer te.wait.Done()

	st := te.newTask(ctx, txn, "register")
	if te.isClosed() {
		st.finish(ErrExecutorClosed)
		return st.future()
	}

	err := te.cfg.Store.RegisterBranches(ctx, txn.Branches)
	st.finish(err)
	return st.future()
}

// state : prepared -> committing
func (te *TccExecutor) Commit(ctx context.Context, txn *model.Txn) *ActionFuture {
	return te.endTxn(ctx, txn, define.BranchTypeConfirm, define.TxnStateCommitting)
}

// state : prepared -> rolling
func (te *TccExecutor) Rollback(ctx context.Context, txn *model.Txn) *ActionFuture {
	return te.endTxn(ctx, txn, define.BranchTypeCancel, define.TxnStateRolling)
}

func (te *TccExecutor) endTxn(ctx context.Context, txn *model.Txn, branchType, state string) *ActionFuture {
	te.wait.Add(1)
	defer te.wait.Done()

	st := te.newTask(ctx, txn, state)
	if te.isClosed() {
		st.finish(ErrExecutorClosed)
		return st.future()
	}

	err := te.startTask(st)
	if err != nil {
		st.finish(err)
		return st.future()
	}

	dbTxn, err := te.cfg.Store.GetByGtid(ctx, txn.Gtid)
	if err != nil {
		te.finishTask(st, err)
		return st.future()
	}

	st.Txn = dbTxn
	te.setTxnLease(st, branchType)
	err = te.stateTransition(st, []string{define.TxnStatePrepared}, state, true)
	if err != nil {
		te.finishTask(st, err)
		return st.future()
	}

	err = te.queueTask(st)
	if err != nil {
		te.finishTask(st, err)
	}
	return st.future()
}

func (te *TccExecutor) process(tcc *actionTask) {
	defer processTimer.Timer()(te.txnType, "process")

	te.wait.Add(1)
	defer te.wait.Done()

	select {
	case <-tcc.Ctx.Done():
		tcc.notify(ErrTimeout)
	case <-te.closeChan:
		te.finishTask(tcc, ErrExecutorClosed)
		return
	default:
	}

	stateGauge.Set(float64(len(te.taskChan)), te.txnType, "inflight")

	if tcc.State == define.TxnStateCommitting {
		te.processAction(tcc, define.BranchTypeConfirm, define.TxnStateCommitting)
	} else if tcc.State == define.TxnStateRolling {
		te.processAction(tcc, define.BranchTypeCancel, define.TxnStateRolling)
	} else if slice.InSlice(tcc.State, define.TxnStateCommitted, define.TxnStateAborted) {
		// state of database might be committing or rolling
		err := te.changeState(tcc, []string{define.TxnStateCommitting, define.TxnStateRolling, tcc.State}, tcc.State, false)
		if err != nil {
			return
		}
		te.finishTask(tcc, nil)
	} else { // prepared
		// notice, clock skew between db and server
		if tcc.ExpireTime.Before(time.Now()) {
			err := te.changeState(tcc, []string{define.TxnStatePrepared, define.TxnStateRolling}, define.TxnStateRolling, true)
			if err != nil {
				return
			}
			te.processAction(tcc, define.BranchTypeCancel, define.TxnStateRolling)
		}
	}
}

func (te *TccExecutor) changeState(task *actionTask, srcStates []string, state string, getUpdatedTxn bool) error {
	err := te.stateTransition(task, srcStates, state, getUpdatedTxn)
	if err == nil {
		return nil
	}
	if !te.finishTaskFatalError(task, err) {
		te.schedule(task, te.cfg.Store.Timeout())
	}
	logutil.Logger(task.Ctx).Sugar().Errorf("state transition : gtid(%s), src(%v), state(%v), error(%v)", task.Gtid, srcStates, state, err)
	return err
}

func (te *TccExecutor) grantBranchLease(task *actionTask, branch *model.Branch, states []string) error {
	duration := branch.Timeout + te.cfg.Store.Timeout()
	if branch.Retry.Constant != nil {
		duration += branch.Retry.Constant.Duration
	}

	task.SetLessee(te.cfg.Lessee)
	task.SetLeaseExpireTime(time.Now().Add(duration))
	err := te.cfg.Store.GrantLeaseIncBranchCheckState(task.Ctx, task.Txn, branch, duration, states)
	if err != nil {
		return err
	}
	branch.TryCount += 1
	return nil
}

func (te *TccExecutor) setTxnLease(task *actionTask, branchType string) {
	task.SetLessee(te.cfg.Lessee)
	expire := time.Now()
	for _, bb := range task.Branches {
		if bb.BranchType == branchType {
			expire.Add(bb.Timeout + bb.RetryDuration())
			break
		}
	}
	task.SetLeaseExpireTime(expire)
}

func (te *TccExecutor) processAction(tcc *actionTask, branchType string, state string) {
	defer processTimer.Timer()(te.txnType, "process_"+branchType)

	for _, branch := range tcc.Branches {
		if branch.BranchType == branchType && branch.State == define.TxnStatePrepared {
			err := te.grantBranchLease(tcc, branch, []string{state})
			if err != nil {
				te.finishTask(tcc, err)
				return
			}

			resp, err := te.action(tcc, branch)
			if err != nil {
				logutil.Logger(context.TODO()).Sugar().Errorf("process action error : %s %d %s %+v\n", tcc.Gtid, branch.Bid, branchType, err)
				te.schedule(tcc, branch.RetryDuration())
				return
			}
			branch.SetResponse(resp)
			branch.SetState(define.TxnStateCommitted)
		}
	}

	newState := operator.IfElse(branchType == define.BranchTypeConfirm, define.TxnStateCommitted, define.TxnStateAborted).(string)
	fromStates := []string{define.TxnStateCommitting, define.TxnStateRolling, newState}
	err := te.changeState(tcc, fromStates, newState, false)
	if err != nil {
		return
	}

	te.finishTask(tcc, nil)
}

func (te *TccExecutor) action(tcc *actionTask, branch *model.Branch) ([]byte, error) {
	branch.IncrTryCount()
	ctx, cancel := context.WithTimeout(tcc.Ctx, branch.Timeout)
	defer cancel()

	payload := branch.Payload
	if len(payload) == 0 && branch.BranchType == define.BranchTypeCancel {
		for _, bb := range tcc.Branches {
			if bb.Bid == branch.Bid && bb.BranchType == define.BranchTypeConfirm {
				payload = bb.Payload
			}
		}
	}

	timer := branchTimer.Timer()
	nf := NewActionNotify(ctx, tcc.Txn, branch.BranchType, branch.Bid, branch.Action, payload)

	te.notifyChan <- nf
	select {
	case err := <-nf.DoneChan:
		timer(te.txnType, branch.BranchType)
		return nf.Msg, err
	case <-te.closeChan:
		timer(te.txnType, branch.BranchType)
		return nil, ErrExecutorClosed
	}
}
