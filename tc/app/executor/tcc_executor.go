package executor

import (
	"context"
	"time"

	"github.com/ikenchina/octopus/common/errorutil"
	logutil "github.com/ikenchina/octopus/common/log"
	"github.com/ikenchina/octopus/common/metrics"
	"github.com/ikenchina/octopus/common/operator"
	"github.com/ikenchina/octopus/common/slice"
	"github.com/ikenchina/octopus/define"
	"github.com/ikenchina/octopus/tc/app/model"
)

var (
	tccBranchTimer = metrics.NewTimer("dtx", "tcc_branch", "tcc branch timer", []string{"branch"})
	tccGauge       = metrics.NewGaugeVec("dtx", "tcc", "in flight tccs", []string{"state"})
)

type TccExecutor struct {
	*baseExecutor
}

func NewTccExecutor(cfg Config) (*TccExecutor, error) {
	te := &TccExecutor{}
	te.baseExecutor = &baseExecutor{
		cfg:     cfg,
		process: te.process,
	}
	return te, nil
}

func (se *TccExecutor) Start() (err error) {
	err = se.baseExecutor.start()
	if err != nil {
		return
	}
	go se.startCrontab()
	go se.startCleanup(define.TxnTypeSaga)
	return
}

func (se *TccExecutor) Stop() error {
	return se.baseExecutor.stop()
}

func (te *TccExecutor) startCrontab() {
	defer errorutil.Recovery()

	limit := 10
	te.cronjob(func() time.Duration {
		duration := te.cronDuration
		tasks, err := te.cfg.Store.FindExpired(context.TODO(), define.TxnTypeTcc, limit)
		if err != nil {
			logutil.Logger(context.Background()).Sugar().Errorf("Find tcc expired transactions error : %v", err)
		}

		tasks2, err := te.cfg.Store.FindLeaseExpired(context.TODO(), define.TxnTypeTcc,
			[]string{define.TxnStateCommitting, define.TxnStateFailed}, limit)
		if err != nil {
			return duration
		}
		tasks = append(tasks, tasks2...)

		for _, task := range tasks {
			t := te.newTask(context.Background(), task)
			if te.startTask(t) != nil {
				continue
			}
			te.queueTask(t)
		}
		if len(tasks) == limit {
			duration = time.Millisecond * 5
		}
		return duration
	})
}

func (te *TccExecutor) newTask(ctx context.Context, txn *model.Txn) *actionTask {
	t := newTask(ctx, txn, func(s, e time.Time, err error) {
		tccGauge.Set(float64(te.taskCount()), "inflight")
	})
	return t
}

func (te *TccExecutor) Get(ctx context.Context, gtid string) (*model.Txn, error) {
	return te.getTask(ctx, gtid)
}

func (te *TccExecutor) Prepare(ctx context.Context, txn *model.Txn) *ActionFuture {
	te.wait.Add(1)
	defer te.wait.Done()
	txn.Branches = nil
	st := te.newTask(ctx, txn)
	if te.isClosed() {
		st.finish(ErrExecutorClosed)
		return st.future()
	}

	err := te.cfg.Store.Save(ctx, txn)
	st.finish(err)
	return st.future()
}

func (te *TccExecutor) Register(ctx context.Context, txn *model.Txn) *ActionFuture {
	te.wait.Add(1)
	defer te.wait.Done()

	st := te.newTask(ctx, txn)
	if te.isClosed() {
		st.finish(ErrExecutorClosed)
		return st.future()
	}

	err := te.cfg.Store.RegisterBranches(ctx, txn.Branches)
	st.finish(err)
	return st.future()
}

func (te *TccExecutor) Commit(ctx context.Context, txn *model.Txn) *ActionFuture {
	return te.endTxn(ctx, txn, define.TxnStateCommitting)
}

func (te *TccExecutor) Rollback(ctx context.Context, txn *model.Txn) *ActionFuture {
	return te.endTxn(ctx, txn, define.TxnStateFailed)
}

func (te *TccExecutor) endTxn(ctx context.Context, txn *model.Txn, state string) *ActionFuture {
	te.wait.Add(1)
	defer te.wait.Done()

	st := te.newTask(ctx, txn)
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

	st.Txn.SetState(state)
	te.setTxnLease(st, operator.IfElse(state == define.TxnStateCommitting,
		define.BranchTypeConfirm, define.BranchTypeCancel).(string))
	err = te.cfg.Store.UpdateConditions(st.Ctx, st.Txn, func(old_txn *model.Txn) error {
		switch old_txn.State {
		case define.TxnStateCommitting, define.TxnStateFailed:
			return ErrInProgress
		case define.TxnStateAborted:
			return ErrStateIsAborted
		case define.TxnStateCommitted:
			return ErrStateIsCommitted
		}
		return nil
	})
	if err != nil {
		te.finishTask(st, err)
		return st.future()
	}

	te.queueTask(st)
	return st.future()
}

func (te *TccExecutor) process(tcc *actionTask) {
	te.wait.Add(1)
	defer te.wait.Done()

	select {
	case <-tcc.Ctx.Done():
		te.finishTask(tcc, ErrTimeout)
		return
	case <-te.closeChan:
		te.finishTask(tcc, ErrExecutorClosed)
		return
	default:
	}

	tccGauge.Set(float64(len(te.taskChan)), "inflight")

	if tcc.State == define.TxnStateCommitting {
		te.processAction(tcc, define.BranchTypeConfirm)
	} else if tcc.State == define.TxnStateFailed {
		te.processAction(tcc, define.BranchTypeCancel)
	} else if slice.InSlice(tcc.State, define.TxnStateCommitted, define.TxnStateAborted) {
		err := te.saveState(tcc, []string{define.TxnStateCommitting, define.TxnStateFailed, tcc.State}, tcc.State)
		if err != nil {
			return
		}
		te.finishTask(tcc, nil)
	} else {
		// notice, clock skew between db and server
		if tcc.ExpireTime.Before(time.Now()) {
			te.processAction(tcc, define.BranchTypeCancel)
		}
	}
}

func (te *TccExecutor) grantLease(task *actionTask, branch *model.Branch) error {
	duration := branch.Timeout
	if branch.Retry.Constant != nil {
		duration += branch.Retry.Constant.Duration
	}

	task.SetLessee(te.cfg.Lessee)
	task.SetLeaseExpireTime(time.Now().Add(duration))
	err := te.cfg.Store.GrantLeaseIncBranch(task.Ctx, task.Txn, branch, duration)
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
			expire.Add(bb.Timeout)
		}
	}

	task.SetLessee(te.cfg.Lessee)
	task.SetLeaseExpireTime(expire)
}

func (te *TccExecutor) processAction(tcc *actionTask, branchType string) {
	for _, branch := range tcc.Branches {
		if branch.BranchType == branchType && branch.State == define.TxnStatePrepared {

			if branch.BranchType == define.BranchTypeCommit && tcc.ExpireTime.Before(time.Now()) {
				te.schedule(tcc, time.Millisecond*100)
				return
			}

			err := te.grantLease(tcc, branch)
			if err != nil {
				te.finishTask(tcc, err)
				return
			}

			_, err = te.action(tcc, branch)
			if err != nil {
				te.schedule(tcc, branch.RetryDuration())
				return
			}
			branch.SetState(define.TxnStateCommitted)
		}
	}

	tcc.SetState(operator.IfElse(branchType == define.BranchTypeConfirm, define.TxnStateCommitted,
		define.TxnStateAborted).(string))
	expectedStates := []string{define.TxnStateCommitting, define.TxnStateFailed, tcc.State}
	if branchType == define.BranchTypeCancel {
		expectedStates = append(expectedStates, define.TxnStatePrepared)
	}
	err := te.saveState(tcc, expectedStates, tcc.State)
	if err != nil {
		return
	}

	te.finishTask(tcc, nil)
}

func (te *TccExecutor) action(tcc *actionTask, branch *model.Branch) (string, error) {
	branch.IncrTryCount()
	ctx, cancel := context.WithTimeout(tcc.Ctx, branch.Timeout)
	defer cancel()

	timer := tccBranchTimer.Timer()

	nf := NewActionNotify(ctx, tcc.Txn, branch.BranchType, branch.Action, branch.Payload)

	te.notifyChan <- nf
	select {
	case err := <-nf.DoneChan:
		timer(branch.BranchType)
		return nf.Msg, err
	case <-te.closeChan:
		timer(branch.BranchType)
		return "", ErrExecutorClosed
	}
}
