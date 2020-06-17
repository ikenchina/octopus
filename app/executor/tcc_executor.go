package executor

import (
	"context"
	"time"

	"github.com/ikenchina/octopus/app/model"
	"github.com/ikenchina/octopus/common/errorutil"
	logutil "github.com/ikenchina/octopus/common/log"
	"github.com/ikenchina/octopus/common/metrics"
	"github.com/ikenchina/octopus/common/operator"
	"github.com/ikenchina/octopus/common/slice"
)

var (
	tccExecTimer = metrics.NewTimer("dtx", "tcc", "tcc timer", []string{"branch"})
	tccGauge     = metrics.NewGaugeVec("dtx", "tcc", "in flight tccs", []string{"state"})
)

type TccExecutor struct {
	*baseExecutor
}

func NewTccExecutor(store model.ModelStorage, maxConcurrency int, lessee string) (*TccExecutor, error) {
	te := &TccExecutor{}
	te.baseExecutor = newExecutor(te.process, store, maxConcurrency, lessee)
	return te, nil
}

func (se *TccExecutor) Start() (err error) {
	err = se.baseExecutor.start()
	if err != nil {
		return
	}
	go se.startCrontab()
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
		tasks, err := te.store.FindTxnExpired(context.TODO(), model.TxnTypeTcc, limit)
		if err != nil {
			logutil.Logger(context.Background()).Sugar().Errorf("Find tcc expired transactions error : %v", err)
		}

		tasks2, err := te.store.FindLeaseExpired(context.TODO(), model.TxnTypeTcc,
			[]string{model.TxnStateCommitting, model.TxnStateFailed}, limit)
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

	err := te.store.Save(ctx, txn)
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

	err := te.store.RegisterBranches(ctx, txn.Branches)
	st.finish(err)
	return st.future()
}

func (te *TccExecutor) Commit(ctx context.Context, txn *model.Txn) *ActionFuture {
	return te.endTxn(ctx, txn, model.TxnStateCommitting)
}

func (te *TccExecutor) Rollback(ctx context.Context, txn *model.Txn) *ActionFuture {
	return te.endTxn(ctx, txn, model.TxnStateFailed)
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

	dbTxn, err := te.store.GetByGtid(ctx, txn.Gtid)
	if err != nil {
		te.finishTask(st, err)
		return st.future()
	}
	st.Txn = dbTxn

	st.Txn.SetState(state)
	te.setTxnLease(st, operator.IfElse(state == model.TxnStateCommitting,
		model.BranchTypeConfirm, model.BranchTypeCancel).(string))
	err = te.store.UpdateConditions(st.Ctx, st.Txn, func(old_txn *model.Txn) error {
		switch old_txn.State {
		case model.TxnStateCommitting, model.TxnStateFailed:
			return ErrInProgress
		case model.TxnStateAborted:
			return ErrStateIsAborted
		case model.TxnStateCommitted:
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

	if tcc.State == model.TxnStateCommitting {
		te.processAction(tcc, model.BranchTypeConfirm)
	} else if tcc.State == model.TxnStateFailed {
		te.processAction(tcc, model.BranchTypeCancel)
	} else if slice.InSlice(tcc.State, model.TxnStateCommitted, model.TxnStateAborted) {
		err := te.saveState(tcc, []string{model.TxnStateCommitting, model.TxnStateFailed, tcc.State}, tcc.State)
		if err != nil {
			return
		}
		te.finishTask(tcc, nil)
	} else {
		// notice, clock skew between db and server
		if tcc.ExpireTime.Before(time.Now()) {
			te.processAction(tcc, model.BranchTypeCancel)
		}
	}
}

func (te *TccExecutor) grantLease(task *actionTask, branch *model.Branch) error {
	duration := branch.Timeout
	if branch.Retry.Constant != nil {
		duration += branch.Retry.Constant.Duration
	}

	task.SetLessee(te.Lessee)
	task.SetLeaseExpireTime(time.Now().Add(duration))
	err := te.store.GrantLeaseIncBranch(task.Ctx, task.Txn, branch, duration)
	if err != nil {
		return err
	}
	branch.TryCount += 1
	return nil
}

func (te *TccExecutor) setTxnLease(task *actionTask, branchType string) {
	task.SetLessee(te.Lessee)
	expire := time.Now()
	for _, bb := range task.Branches {
		if bb.BranchType == branchType {
			expire.Add(bb.Timeout)
		}
	}

	task.SetLessee(te.Lessee)
	task.SetLeaseExpireTime(expire)
}

func (te *TccExecutor) processAction(tcc *actionTask, branchType string) {
	for _, branch := range tcc.Branches {
		if branch.BranchType == branchType && branch.State == model.TxnStatePrepared {

			if branch.BranchType == model.BranchTypeCommit && tcc.ExpireTime.Before(time.Now()) {
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
			branch.SetState(model.TxnStateCommitted)
		}
	}

	tcc.SetState(operator.IfElse(branchType == model.BranchTypeConfirm, model.TxnStateCommitted,
		model.TxnStateAborted).(string))
	expectedStates := []string{model.TxnStateCommitting, model.TxnStateFailed, tcc.State}
	if branchType == model.BranchTypeCancel {
		expectedStates = append(expectedStates, model.TxnStatePrepared)
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

	timer := tccExecTimer.Timer()

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
