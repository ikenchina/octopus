package executor

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/ikenchina/octopus/define"
	"github.com/ikenchina/octopus/tc/app/model"
)

func TestSagaSuite(t *testing.T) {
	suite.Run(t, new(_sagaSuite))
}

// normal case
func (s *_sagaSuite) TestNormal() {
	storage, sagaExec, sagaNotifier, _ := s.startSagaExecutor(nil)
	ctx := context.Background()

	sagaPm := s.newSage()
	sagaTask := sagaExec.Commit(ctx, sagaPm)

	// response checking
	s.Nil(sagaTask.GetError())
	s.checkState(define.TxnStateCommitted, sagaPm)

	// check storage model
	s.checkStorage(storage, sagaTask.Txn)

	// participant
	for _, branch := range sagaPm.Branches {
		if branch.BranchType != s.commitBranch {
			continue
		}
		h, ok := s.servers.handlers[branch.Action]
		s.True(ok)
		s.Equal(1, h.requestCount())
	}

	// notify
	s.Equal(1, sagaNotifier.requestCount())
	s.Equal(sagaNotifier.requestCount(), sagaTask.NotifyCount)
	s.Nil(sagaExec.Stop())
}

// test timeout and retry
// 3 participants,
//     2nd participant : timeout for first time, ok for retry
func (s *_sagaSuite) TestTimeoutRetryOk() {
	storage, sagaExec, notifier, _ := s.startSagaExecutor(nil)
	ctx := context.Background()
	sagaPm := s.newSage()

	// timeout conditions
	service1Handler := s.getServerHandlers(1, define.BranchTypeCommit)
	service0Commit := s.getServerBranch(0, sagaPm)[0]
	service1Commit := s.getServerBranch(1, sagaPm)[0]
	service2Commit := s.getServerBranch(2, sagaPm)[0]
	service1Commit.Retry.MaxRetry = 1
	service1Commit.Retry.Constant.Duration = 10 * time.Millisecond
	service1Handler.setTimeout(30 * time.Millisecond)

	sagaFuture := sagaExec.Commit(ctx, sagaPm)

	// ############   first time   ############
	// processing, blocking by 2nd participant
	time.Sleep(time.Millisecond * 15)

	// check storage model
	s.Equal(define.TxnStatePrepared, sagaPm.State)
	s.Equal(1, service0Commit.TryCount)
	s.Equal(1, service1Commit.TryCount)
	s.Equal(0, service2Commit.TryCount)

	service1Handler.setTimeout(10 * time.Millisecond)
	time.Sleep(time.Millisecond * (10 + 10)) // timeout, rescheduling

	// ############   second time   ############
	s.Equal(define.TxnStateFailed, service1Commit.State)

	// response checking
	s.Nil(sagaFuture.GetError())
	s.checkState(define.TxnStateCommitted, sagaFuture.Txn)

	// check storage model
	s.checkStorage(storage, sagaFuture.Txn)

	// participant
	for url, rm := range s.servers.handlers {
		if rm.name != s.commitBranch {
			continue
		}
		rr := s.getServerBranch(0, sagaPm)[0]
		s.Equal(rm.requestCount(), rr.TryCount)
		if url == service1Commit.Action {
			s.Equal(2, rm.requestCount())
		} else {
			s.Equal(1, rm.requestCount())
		}
	}

	// notify
	s.Equal(1, notifier.requestCount())
	s.Equal(notifier.requestCount(), sagaFuture.NotifyCount)

	s.Nil(sagaExec.Stop())
}

// test roll back
func (s *_sagaSuite) TestCompensationOk() {

	storage, sagaExec, notifier, _ := s.startSagaExecutor(nil)

	ctx := context.Background()
	sagaPm := s.newSage()

	// timeout conditions
	service1Handler := s.getServerHandlers(1, define.BranchTypeCommit)
	service0Commit := s.getServerBranch(0, sagaPm)[0]
	service1Commit := s.getServerBranch(1, sagaPm)[0]
	service2Commit := s.getServerBranch(2, sagaPm)[0]
	service0Rollback := s.getServerBranch(0, sagaPm)[1]
	service1Rollback := s.getServerBranch(1, sagaPm)[1]
	service2Rollback := s.getServerBranch(2, sagaPm)[1]

	// timeout conditions
	service1Commit.Retry.MaxRetry = 1
	service1Commit.Retry.Constant.Duration = 10 * time.Millisecond
	service1Handler.setTimeout(30 * time.Millisecond)

	sagaFuture := sagaExec.Commit(ctx, sagaPm)

	// ############   first time   ############
	// processing, blocking by 2nd participant
	time.Sleep(time.Millisecond * 15)

	// check storage model
	get, err := storage.GetByGtid(ctx, sagaFuture.Gtid)
	s.Nil(err)
	s.Equal(define.TxnStatePrepared, get.State)
	s.Equal(1, service0Commit.TryCount)
	s.Equal(1, service1Commit.TryCount)
	s.Equal(0, service2Commit.TryCount)

	time.Sleep(time.Millisecond * (15 + 5)) // timeout, rescheduling

	// ############   second time   ############

	_, err = storage.GetByGtid(ctx, sagaFuture.Gtid)
	s.Nil(err)
	s.Equal(define.TxnStateFailed, service1Commit.State)

	// response checking
	s.Nil(sagaFuture.GetError())
	s.Equal(define.TxnStateAborted, sagaFuture.Txn.State)

	// check storage model
	s.checkStorage(storage, sagaFuture.Txn)

	// participant
	s.Equal(1, service0Commit.TryCount)
	s.Equal(2, service1Commit.TryCount)
	s.Equal(0, service2Commit.TryCount)
	s.Equal(1, service2Rollback.TryCount) // avoid disorder issue
	s.Equal(1, service1Rollback.TryCount)
	s.Equal(1, service0Rollback.TryCount)

	for _, bb := range sagaPm.Branches {
		if bb.BranchType == s.commitBranch || bb.BranchType == define.BranchTypeCompensation {
			h := s.servers.getHandlerByAction(bb.Action)
			s.Equal(h.requestCount(), bb.TryCount)
		}
	}

	// notify
	s.Equal(1, int(notifier.count))
	s.Equal(int(notifier.count), sagaFuture.NotifyCount)

	nr := notifier.getNotify(sagaFuture.Gtid)
	s.Equal(define.TxnStateAborted, nr.State)

	s.Nil(sagaExec.Stop())
}

func (s *_sagaSuite) TestCommitError() {

	_, sagaExec, _, _ := s.startSagaExecutor(nil)

	ctx := context.Background()
	sagaPm := s.newSage()

	sagaFuture1 := sagaExec.Commit(ctx, sagaPm)
	sagaFuture2 := sagaExec.Commit(ctx, sagaPm)

	s.Nil(sagaFuture1.GetError())
	s.Equal(ErrInProgress, sagaFuture2.GetError())
}

// func (s *_sagaSuite) TestNotify() {

// }

// func (s *_sagaSuite) TestConcurrencyLimit() {

// }

// func (s *_sagaSuite) TestLeasse() {

// }

func (s *_sagaSuite) TestCron() {
	storage, sagaExec, _, _ := s.startSagaExecutor(nil)
	ctx := context.Background()
	sagaPm := s.newSage()

	// timeout conditions
	service1Handler := s.getServerHandlers(1, define.BranchTypeCommit)
	service1Commit := s.getServerBranch(1, sagaPm)[0]
	service1Commit.Retry.MaxRetry = 1
	service1Commit.Retry.Constant.Duration = 10 * time.Millisecond
	service1Handler.setTimeout(30 * time.Millisecond)

	sagaFuture := sagaExec.Commit(ctx, sagaPm)
	sagaFuture2 := sagaExec.Commit(ctx, s.newSage())
	time.Sleep(20 * time.Millisecond)
	s.Nil(sagaExec.Stop())

	// expired
	time.Sleep(100 * time.Millisecond)

	sg1, _ := sagaExec.Get(ctx, sagaFuture.Gtid)
	sg2, _ := sagaExec.Get(ctx, sagaFuture2.Gtid)
	s.Equal(define.TxnStatePrepared, sg1.State)
	s.Equal(define.TxnStatePrepared, sg2.State)

	service1Handler.setTimeout(10 * time.Millisecond)

	_, sagaExec2, notifier2, _ := s.startSagaExecutor(storage)
	time.Sleep(500 * time.Millisecond)

	sg1, _ = sagaExec.Get(ctx, sagaFuture.Gtid)
	sg2, _ = sagaExec.Get(ctx, sagaFuture2.Gtid)
	s.Equal(define.TxnStateCommitted, sg1.State)
	s.Equal(define.TxnStateCommitted, sg2.State)
	s.Equal(define.TxnStateCommitted, notifier2.getNotify(sg1.Gtid).State)
	s.Equal(define.TxnStateCommitted, notifier2.getNotify(sg2.Gtid).State)
	s.Nil(sagaExec2.Stop())
}

////////////////////////////////////////////////
////////////////////////////////////////////////
////////////////////////////////////////////////

type _sagaSuite struct {
	_baseSuite
}

func (s *_sagaSuite) newSage() *model.Txn {
	s.tidCounter++
	now := time.Now()
	sg := &model.Txn{
		Gtid:              s.newTid(),
		Business:          "test_biz",
		TxnType:           define.TxnTypeSaga,
		State:             define.TxnStatePrepared,
		UpdatedTime:       now,
		CreatedTime:       now,
		ExpireTime:        now.Add(60 * time.Second),
		Lessee:            s.lessee,
		CallType:          define.TxnCallTypeSync,
		ParallelExecution: false,
		NotifyAction:      s.notify,
		NotifyTimeout:     time.Millisecond * 20,
		NotifyRetry:       time.Millisecond * 10,
	}

	names := []string{"service0", "service1", "service2"}
	types := []string{s.commitBranch, define.BranchTypeCompensation}
	for _, branch := range names {
		for _, tt := range types {
			hh := s.servers.getHandler(branch, tt)

			s.tidCounter++
			sub := &model.Branch{
				Gtid:        sg.Gtid,
				Bid:         s.tidCounter,
				BranchType:  tt,
				Action:      hh.url,
				Payload:     s.servers.newRequestBody(sg.Gtid),
				Timeout:     hh.requestTimeout * 2,
				State:       define.TxnStatePrepared,
				UpdatedTime: now,
				CreatedTime: now,
				TryCount:    0,
			}

			sub.Retry.MaxRetry = 2
			sub.Retry.Constant = &model.RetryConstant{
				Duration: time.Millisecond * 10,
			}
			sg.Branches = append(sg.Branches, sub)
		}
	}

	return sg
}

func (s *_sagaSuite) checkState(state string, sm *model.Txn) {
	s.Equal(state, sm.State)
	for idx, sub := range sm.Branches {
		if idx&1 == 0 {
			s.Equal(state, sub.State)
		}
	}
}

func (s *_sagaSuite) startSagaExecutor(storage model.ModelStorage) (model.ModelStorage, *SagaExecutor, *rmMockHandler, *rmClientMock) {

	s.initServers(define.TxnTypeSaga)
	if storage == nil {
		storage = model.NewModelStorageMock(s.lessee)
	}

	sagaExec, err := NewSagaExecutor(Config{
		Store:          storage,
		MaxConcurrency: 10,
		Lessee:         s.lessee,
		CleanExpired:   time.Second,
		CleanLimit:     10,
	})

	s.NotNil(sagaExec)
	s.Nil(err)
	s.Nil(sagaExec.Start())

	client := &rmClientMock{servers: s.servers}
	client.start(sagaExec.NotifyChan())

	notifier := &rmMockHandler{
		requestTimeout: time.Millisecond * 1,
	}
	s.servers.handlers[s.notify] = notifier

	return storage, sagaExec, notifier, client
}
