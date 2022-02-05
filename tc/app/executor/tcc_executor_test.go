package executor

import (
	"context"
	"testing"
	"time"

	"github.com/ikenchina/octopus/define"
	"github.com/ikenchina/octopus/tc/app/model"
	"github.com/stretchr/testify/suite"
)

func TestTccSuite(t *testing.T) {
	suite.Run(t, new(_tccSuite))
}

// normal case
func (s *_tccSuite) TestNormal() {
	storage, exec, _, _ := s.startExecutor(nil)
	ctx := context.Background()

	txn1 := s.newTxn()
	branches1 := txn1.Branches
	txn1.Branches = nil

	p1Future := exec.Prepare(ctx, txn1)
	s.Nil(p1Future.GetError())

	for _, branch := range branches1 {
		txn1.Branches = []*model.Branch{branch}
		rbf := exec.Register(ctx, txn1)
		s.Nil(rbf.GetError())
	}

	cf := exec.Commit(ctx, txn1)
	s.Nil(cf.GetError())
	txn1 = cf.Txn

	s.checkCommitState(define.TxnStateCommitted, txn1)

	// check storage model
	s.checkStorage(storage, txn1)

	// participant
	for _, branch := range txn1.Branches {
		if branch.BranchType != s.commitBranch {
			continue
		}
		h, ok := s.servers.handlers[branch.Action]
		s.True(ok)
		s.Equal(1, h.requestCount())
	}

	s.Nil(exec.Stop())
}

func (s *_tccSuite) TestCancel() {
	storage, exec, _, _ := s.startExecutor(nil)
	ctx := context.Background()

	txn1 := s.newTxn()
	branches1 := txn1.Branches
	txn1.Branches = nil
	p1Future := exec.Prepare(ctx, txn1)
	s.Nil(p1Future.GetError())

	for _, branch := range branches1 {
		txn1.Branches = []*model.Branch{branch}
		rbf := exec.Register(ctx, txn1)
		s.Nil(rbf.GetError())
	}

	cf := exec.Rollback(ctx, txn1)
	s.Nil(cf.GetError())
	txn1 = cf.Txn
	s.checkRollbackState(define.TxnStateAborted, txn1)
	s.checkStorage(storage, txn1)
	for _, branch := range txn1.Branches {
		h, ok := s.servers.handlers[branch.Action]
		s.True(ok)
		if branch.BranchType == s.rollbackBranch {
			s.Equal(1, h.requestCount())
		} else {
			s.Equal(0, h.requestCount())
		}
	}
	s.Nil(exec.Stop())
}

func (s *_tccSuite) TestCancelRetry() {
	_, exec, _, _ := s.startExecutor(nil)
	ctx := context.Background()

	txn1 := s.newTxn()
	branches1 := txn1.Branches
	service1 := s.getServerHandlers(1, define.BranchTypeCancel)
	service1.setTimeout(40 * time.Millisecond)
	txn1.Branches = nil

	p1Future := exec.Prepare(ctx, txn1)
	s.Nil(p1Future.GetError())

	for _, branch := range branches1 {
		txn1.Branches = []*model.Branch{branch}
		rbf := exec.Register(ctx, txn1)
		s.Nil(rbf.GetError())
	}

	go func() {
		cf := exec.Rollback(ctx, txn1)
		s.Nil(cf.GetError())
	}()

	time.Sleep(500 * time.Millisecond)
	txn1, err := exec.Get(ctx, txn1.Gtid)
	s.Nil(err)

	service1.setTimeout(5 * time.Millisecond)
	time.Sleep(100 * time.Millisecond)

	txn1, err = exec.Get(ctx, txn1.Gtid)
	s.Nil(err)
	s.checkRollbackState(define.TxnStateAborted, txn1)
	for _, branch := range txn1.Branches {
		h, ok := s.servers.handlers[branch.Action]
		s.True(ok)
		if branch.BranchType == s.rollbackBranch {
			if branch.Action == service1.url {
				s.Greater(h.requestCount(), 4)
			} else {
				s.Equal(1, h.requestCount())
			}
		}
	}
	s.Nil(exec.Stop())
}

func (s *_tccSuite) TestExpiredCancel() {
	storage, exec, _, _ := s.startExecutor(nil)
	ctx := context.Background()
	exec.setCronDuration(20 * time.Millisecond)

	txn1 := s.newTxn()
	txn1.ExpireTime = time.Now().Add(200 * time.Millisecond)
	branches1 := txn1.Branches
	txn1.Branches = nil
	p1Future := exec.Prepare(ctx, txn1)
	s.Nil(p1Future.GetError())

	for _, branch := range branches1 {
		txn1.Branches = []*model.Branch{branch}
		rbf := exec.Register(ctx, txn1)
		s.Nil(rbf.GetError())
	}
	time.Sleep(400 * time.Millisecond)
	txn1, err := exec.Get(ctx, txn1.Gtid)
	s.Nil(err)

	s.checkRollbackState(define.TxnStateAborted, txn1)
	s.checkStorage(storage, txn1)
	for _, branch := range txn1.Branches {
		h, ok := s.servers.handlers[branch.Action]
		s.True(ok)
		if branch.BranchType == s.rollbackBranch {
			s.Equal(1, h.requestCount())
		} else {
			s.Equal(0, h.requestCount())
		}
	}

	s.Nil(exec.Stop())
}

func (s *_tccSuite) TestExpiredConfirm() {
	storage, exec, _, _ := s.startExecutor(nil)
	ctx := context.Background()
	exec.setCronDuration(20 * time.Millisecond)

	txn1 := s.newTxn()
	txn1.ExpireTime = time.Now().Add(100 * time.Millisecond)
	txn1.SetState(define.TxnStateCommitting)
	s.Nil(storage.Save(ctx, txn1))

	time.Sleep(200 * time.Millisecond) // scheduler : find expired txn every second
	txn1, err := exec.Get(ctx, txn1.Gtid)
	s.Nil(err)

	s.checkCommitState(define.TxnStateCommitted, txn1)
	for _, branch := range txn1.Branches {
		h, ok := s.servers.handlers[branch.Action]
		s.True(ok)
		if branch.BranchType == s.commitBranch {
			s.Equal(1, h.requestCount())
		} else {
			s.Equal(0, h.requestCount())
		}
	}
	s.Nil(exec.Stop())
}

func (s *_tccSuite) TestLeaseExpiredConfirm() {
	storage, exec, _, _ := s.startExecutor(nil)
	ctx := context.Background()
	exec.setCronDuration(20 * time.Millisecond)

	txn1 := s.newTxn()
	txn1.ExpireTime = time.Now().Add(40000 * time.Millisecond)
	txn1.SetState(define.TxnStateCommitting)
	txn1.SetLeaseExpireTime(time.Now().Add(40 * time.Millisecond))
	s.Nil(storage.Save(ctx, txn1))

	time.Sleep(100 * time.Millisecond) // scheduler : find expired txn every second
	txn1, err := exec.Get(ctx, txn1.Gtid)
	s.Nil(err)

	s.checkCommitState(define.TxnStateCommitted, txn1)
	for _, branch := range txn1.Branches {
		h, ok := s.servers.handlers[branch.Action]
		s.True(ok)
		if branch.BranchType == s.commitBranch {
			s.Equal(1, h.requestCount())
		} else {
			s.Equal(0, h.requestCount())
		}
	}
	s.Nil(exec.Stop())
}

func (s *_tccSuite) TestTimeout() {
	_, exec, _, _ := s.startExecutor(nil)
	ctx := context.Background()

	txn1 := s.newTxn()
	branches1 := txn1.Branches
	service1 := s.getServerHandlers(1, define.BranchTypeConfirm)
	service1Commit := s.getServerBranch(1, txn1)[0]
	service1.setTimeout(20 * time.Millisecond)
	service1Commit.Timeout = 10 * time.Millisecond
	service1Commit.Retry.Constant.Duration = 10 * time.Millisecond
	txn1.Branches = nil

	p1Future := exec.Prepare(ctx, txn1)
	s.Nil(p1Future.GetError())

	for _, branch := range branches1 {
		txn1.Branches = []*model.Branch{branch}
		rbf := exec.Register(ctx, txn1)
		s.Nil(rbf.GetError())
	}

	go func() {
		cf := exec.Commit(ctx, txn1)
		s.Nil(cf.GetError())
	}()

	time.Sleep(30 * time.Millisecond)
	service1.setTimeout(5 * time.Millisecond)
	time.Sleep(100 * time.Millisecond)

	txn1, err := exec.Get(ctx, txn1.Gtid)
	s.Nil(err)
	s.checkCommitState(define.TxnStateCommitted, txn1)
	for _, branch := range txn1.Branches {
		h, ok := s.servers.handlers[branch.Action]
		s.True(ok)
		if branch.BranchType == s.commitBranch {
			if branch.Action == service1Commit.Action {
				s.Equal(2, h.requestCount())
			} else {
				s.Equal(1, h.requestCount())
			}
		} else {
			s.Equal(0, h.requestCount())
		}
	}
	s.Nil(exec.Stop())
}

///
///
///

type _tccSuite struct {
	_baseSuite
}

func (s *_tccSuite) checkCommitState(state string, sm *model.Txn) {
	s.Equal(state, sm.State)
	for _, bb := range sm.Branches {
		if bb.BranchType == s.commitBranch {
			s.Equal(define.TxnStateCommitted, bb.State)
		}
	}
}

func (s *_tccSuite) checkRollbackState(state string, sm *model.Txn) {
	s.Equal(state, sm.State)
	for _, bb := range sm.Branches {
		if bb.BranchType == s.rollbackBranch {
			s.Equal(define.TxnStateCommitted, bb.State)
		}
	}
}

func (s *_tccSuite) startExecutor(storage model.ModelStorage) (model.ModelStorage, *TccExecutor, *rmMockHandler, *rmClientMock) {
	s.initServers(define.TxnTypeTcc)
	if storage == nil {
		storage = model.NewModelStorageMock(s.lessee)
	}
	exec, err := NewTccExecutor(Config{
		Store:          storage,
		MaxConcurrency: 10,
		Lessee:         s.lessee,
		CleanExpired:   time.Second,
		CleanLimit:     10,
	})
	s.NotNil(exec)
	s.Nil(err)
	s.Nil(exec.Start())
	client := &rmClientMock{servers: s.servers}
	client.start(exec.NotifyChan())
	notifier := &rmMockHandler{}
	s.servers.handlers[s.notify] = notifier
	return storage, exec, notifier, client
}

func (s *_tccSuite) newTxn() *model.Txn {
	s.tidCounter++
	now := time.Now()
	sg := &model.Txn{
		Gtid:              s.newTid(),
		Business:          "test_biz",
		TxnType:           define.TxnTypeTcc,
		State:             define.TxnStatePrepared,
		UpdatedTime:       now,
		CreatedTime:       now,
		ExpireTime:        now.Add(60 * time.Second),
		Lessee:            s.lessee,
		CallType:          define.TxnCallTypeSync,
		ParallelExecution: false,
		NotifyAction:      "http://dtx/notify",
		NotifyTimeout:     time.Millisecond * 10,
		NotifyRetry:       time.Millisecond * 10,
	}

	names := []string{"service0", "service1", "service2"}
	types := []string{define.BranchTypeConfirm, define.BranchTypeCancel}
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
