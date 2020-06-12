package executor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ikenchina/octopus/common/errorutil"
	logutil "github.com/ikenchina/octopus/common/log"
	"github.com/ikenchina/octopus/common/metrics"
	"github.com/ikenchina/octopus/common/slice"
	"github.com/ikenchina/octopus/tc/app/model"
)

var (
	ErrUnknown           = errors.New("unknown")
	ErrTimeout           = errors.New("timeout")
	ErrTaskIsAborted     = errors.New("is aborted")
	ErrTaskAlreadyExist  = errors.New("task already exist")
	ErrNotExists         = errors.New("not exists")
	ErrInProgress        = errors.New("in progress")
	ErrStateIsNotPrepare = errors.New("transaction is not prepare")
	ErrStateIsAborted    = errors.New("transaction is aborted")
	ErrStateIsCommitted  = errors.New("transaction is committed")
	ErrExceedMaxTryCount = errors.New("exceed max try count")
	ErrExecutorClosed    = errors.New("closed")
	ErrInvalidState      = errors.New("invalid state")
)

var (
	txnTimer    = metrics.NewTimer("dtx", "txn", "txn timer", []string{"type", "op"})
	branchTimer = metrics.NewTimer("dtx", "branch", "branch timer", []string{"type", "branch"})
	stateGauge  = metrics.NewGaugeVec("dtx", "txn", "state", []string{"type", "state"})
)

type Config struct {
	Store                     model.ModelStorage
	MaxConcurrency            int
	Lessee                    string
	LeaseExpiredLimit         int
	CleanExpired              time.Duration
	CleanLimit                int
	CheckExpiredDuration      time.Duration
	CheckLeaseExpiredDuration time.Duration
}

func NewActionNotify(ctx context.Context, txn *model.Txn, bt string, action, payload string) *ActionNotify {
	return &ActionNotify{
		txn:        txn,
		BranchType: bt,
		Action:     action,
		Payload:    payload,
		Ctx:        ctx,
		DoneChan:   make(chan error, 1),
	}
}

type ActionNotify struct {
	txn        *model.Txn
	BranchType string
	Action     string
	Payload    string
	Ctx        context.Context
	DoneChan   chan error
	Msg        string
}

func (sn *ActionNotify) GID() string {
	return sn.txn.Gtid
}

func (sn *ActionNotify) Txn() *model.Txn {
	return sn.txn
}

func (sn *ActionNotify) Done(err error, msg string) {
	sn.Msg = msg
	sn.DoneChan <- err
}

type ActionFuture actionTask

func (sf *ActionFuture) Get() <-chan error {
	return sf.errChan
}
func (sf *ActionFuture) GetError() error {
	return <-sf.errChan
}

type actionTask struct {
	*model.Txn
	Ctx       context.Context
	errChan   chan error
	startTime time.Time
	finishCB  func(start, end time.Time, err error)
}

func (s *actionTask) future() *ActionFuture {
	return (*ActionFuture)(s)
}

func (s *actionTask) finish(err error) {
	select {
	case s.errChan <- err:
	default:
	}

	if s.finishCB != nil {
		s.finishCB(s.startTime, time.Now(), err)
	}
}

func newTask(ctx context.Context, txn *model.Txn, finishCB func(start, end time.Time, err error)) *actionTask {
	return &actionTask{
		Txn:       txn,
		Ctx:       ctx,
		errChan:   make(chan error, 1),
		startTime: time.Now(),
		finishCB:  finishCB,
	}
}

//
//
//
//
//
type baseExecutor struct {
	mutex        sync.RWMutex
	closed       int32
	wait         sync.WaitGroup
	closeChan    chan struct{}
	taskChan     chan *actionTask
	tasks        map[string]*actionTask
	notifyChan   chan *ActionNotify
	process      func(t *actionTask)
	cronDuration time.Duration
	cfg          Config
	txnType      string
}

func (ex *baseExecutor) start() error {
	atomic.StoreInt32(&ex.closed, 0)
	ex.closeChan = make(chan struct{})
	ex.taskChan = make(chan *actionTask, ex.cfg.MaxConcurrency*2)
	ex.tasks = make(map[string]*actionTask)
	ex.notifyChan = make(chan *ActionNotify, ex.cfg.MaxConcurrency*4)
	ex.cronDuration = ex.cfg.CheckLeaseExpiredDuration
	if ex.cronDuration == time.Duration(0) {
		ex.cronDuration = time.Second * 1
	}

	for i := 0; i < ex.cfg.MaxConcurrency; i++ {
		ex.wait.Add(1)
		go func() {
			defer errorutil.Recovery()
			defer ex.wait.Done()
			for task := range ex.taskChan {
				ex.process(task)
			}
		}()
	}
	return nil
}

func (ex *baseExecutor) stop() error {
	if atomic.CompareAndSwapInt32(&ex.closed, 0, 1) {
		close(ex.closeChan)
		close(ex.taskChan)
		ex.wait.Wait()
		close(ex.notifyChan)
		ex.clearTasks()
	}
	return nil
}

func (ex *baseExecutor) Closed() bool {
	return atomic.LoadInt32(&ex.closed) == 1
}

func (ex *baseExecutor) setCronDuration(d time.Duration) {
	ex.cronDuration = d
}

func (ex *baseExecutor) cronjob(job func() time.Duration) {
	ex.wait.Add(1)
	defer ex.wait.Done()

	for {
		duration := job()
		select {
		case <-time.After(duration):
		case <-ex.closeChan:
			return
		}
	}
}

func (ex *baseExecutor) startCleanup(txn string) {
	ex.wait.Add(1)
	defer ex.wait.Done()

	maxDuration := ex.cfg.CheckExpiredDuration
	minDuration := time.Millisecond * 100
	clean := func() time.Duration {
		txns, err := ex.cfg.Store.CleanExpiredTxns(context.Background(),
			txn, time.Now().Add(-1*ex.cfg.CleanExpired), ex.cfg.CleanLimit)
		if err != nil {
			return maxDuration
		}
		if len(txns) < ex.cfg.CleanLimit {
			return maxDuration
		}
		return minDuration
	}

	for {
		d := clean()
		select {
		case <-time.After(d):
		case <-ex.closeChan:
			return
		}
	}
}

func (ex *baseExecutor) queueTask(task *actionTask) error {
	ex.wait.Add(1)
	defer ex.wait.Done()

	if ex.isClosed() {
		return ErrExecutorClosed
	}

	select {
	case <-ex.closeChan:
		return ErrExecutorClosed
	case <-task.Ctx.Done():
		return ErrTimeout
	case ex.taskChan <- task:
	}
	return nil
}

func (ex *baseExecutor) taskCount() int {
	ex.mutex.RLock()
	defer ex.mutex.RUnlock()
	return len(ex.tasks)
}

func (ex *baseExecutor) getTask(ctx context.Context, id string) (*model.Txn, error) {
	ex.mutex.RLock()
	st, ok := ex.tasks[id]
	if !ok {
		ex.mutex.RUnlock()
		return ex.cfg.Store.GetByGtid(ctx, id)
	}
	ex.mutex.RUnlock()
	return st.Txn, nil
}

func (ex *baseExecutor) clearTasks() {
	ex.mutex.Lock()
	tasks := ex.tasks
	ex.tasks = make(map[string]*actionTask)
	ex.mutex.Unlock()

	for _, sg := range tasks {
		sg.finish(ErrExecutorClosed)
	}
}

func (ex *baseExecutor) NotifyChan() <-chan *ActionNotify {
	return ex.notifyChan
}

func (ex *baseExecutor) isClosed() bool {
	return atomic.LoadInt32(&ex.closed) == 1
}

func (ex *baseExecutor) finishTask(task *actionTask, err error) {
	task.finish(err)
	ex.mutex.Lock()
	defer ex.mutex.Unlock()
	delete(ex.tasks, task.Gtid)
}

func (ex *baseExecutor) startTask(task *actionTask) error {
	ex.mutex.Lock()
	defer ex.mutex.Unlock()

	_, ok := ex.tasks[task.Gtid]
	if ok {
		return ErrInProgress
	}
	ex.tasks[task.Gtid] = task
	return nil
}

func (ex *baseExecutor) finishTaskFatalError(task *actionTask, err error) bool {
	if err == ErrInvalidState || err == model.ErrInvalidLessee {
		logutil.Logger(task.Ctx).Sugar().Errorf("fatal error : gtid(%s), error(%v)", task.Gtid, err)
		ex.finishTask(task, err)
		return true
	}
	return false
}

func (ex *baseExecutor) saveState(task *actionTask, srcStates []string, state string) error {
	originState := task.State
	task.SetState(state)
	err := ex.cfg.Store.UpdateConditions(task.Ctx, task.Txn, func(oldTxn *model.Txn) error {
		if !slice.Contain(srcStates, oldTxn.State) {
			return fmt.Errorf("src(%v), old(%v)", srcStates, oldTxn.State)
			//return ErrInvalidState
		}
		return nil
	})
	if err == nil {
		return nil
	}

	task.SetState(originState)
	if !ex.finishTaskFatalError(task, err) {
		ex.schedule(task, ex.cfg.Store.Timeout())
	}

	return err
}

func (ex *baseExecutor) schedule(task *actionTask, after time.Duration) {
	if ex.isClosed() {
		ex.finishTask(task, ErrExecutorClosed)
		return
	}

	time.AfterFunc(after, func() {
		deadline, ok := task.Ctx.Deadline()
		if ok && deadline.Before(time.Now().Add(after)) {
			task.finish(ErrTimeout)
			return
		}
		err := ex.queueTask(task)
		if err != nil {
			ex.finishTask(task, err)
		}
	})
}

func LogIfErr(err error, msg string, task *actionTask) {
	if err != nil {
		logutil.Logger(task.Ctx).Sugar().Errorf(msg+" : error(%+v), task(%+v)", err, task)
	}
}
