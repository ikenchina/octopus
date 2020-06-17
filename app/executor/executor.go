package executor

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ikenchina/octopus/app/model"
	"github.com/ikenchina/octopus/common/errorutil"
	logutil "github.com/ikenchina/octopus/common/log"
	"github.com/ikenchina/octopus/common/slice"
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
	mutex          sync.RWMutex
	Lessee         string
	closed         int32
	maxConcurrency int
	wait           sync.WaitGroup
	closeChan      chan struct{}
	taskChan       chan *actionTask
	tasks          map[string]*actionTask
	notifyChan     chan *ActionNotify
	store          model.ModelStorage
	process        func(t *actionTask)
	cronDuration   time.Duration
}

func newExecutor(p func(t *actionTask), store model.ModelStorage, maxConcurrency int, lessee string) *baseExecutor {
	return &baseExecutor{
		Lessee:         lessee,
		maxConcurrency: maxConcurrency,
		store:          store,
		process:        p,
	}
}

func (ex *baseExecutor) start() error {
	atomic.StoreInt32(&ex.closed, 0)
	ex.closeChan = make(chan struct{})
	ex.taskChan = make(chan *actionTask, ex.maxConcurrency*2)
	ex.tasks = make(map[string]*actionTask)
	ex.notifyChan = make(chan *ActionNotify, ex.maxConcurrency*4)

	for i := 0; i < ex.maxConcurrency; i++ {
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

	duration := time.Second * 0
	for {
		duration = job()

		select {
		case <-time.After(duration):
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
		return ex.store.GetByGtid(ctx, id)
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
	err := ex.store.UpdateConditions(task.Ctx, task.Txn, func(oldTxn *model.Txn) error {
		if !slice.Contain(srcStates, oldTxn.State) {
			return ErrInvalidState
		}
		return nil
	})
	if err == nil {
		return nil
	}

	task.SetState(originState)
	if !ex.finishTaskFatalError(task, err) {
		ex.schedule(task, ex.store.Timeout())
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
		ex.queueTask(task)
	})
}

func LogIfErr(err error, msg string, task *actionTask) {
	if err != nil {
		logutil.Logger(task.Ctx).Sugar().Errorf(msg+" : error(%+v), task(%+v)", err, task)
	}
}
