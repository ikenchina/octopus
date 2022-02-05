package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/ikenchina/octopus/common/slice"
	"github.com/ikenchina/octopus/define"
	"github.com/ikenchina/octopus/tc/app/model"
)

type _baseSuite struct {
	suite.Suite
	tidCounter     int
	servers        *rmMock
	serverNames    []string
	lessee         string
	notify         string
	commitBranch   string
	rollbackBranch string
}

func (s *_baseSuite) getServerHandlers(index int, branchType string) *rmMockHandler {
	name := s.serverNames[index]
	for _, h := range s.servers.handlers {
		if h.name == name && h.branch == branchType {
			return h
		}
	}
	return nil
}

func (s *_baseSuite) getServerBranch(index int, saga *model.Txn) []*model.Branch {
	bb := []*model.Branch{}
	urls := []string{}
	name := s.serverNames[index]
	for _, h := range s.servers.handlers {
		if h.name == name {
			urls = append(urls, h.url)
		}
	}
	for _, tt := range saga.Branches {
		if slice.Contain(urls, tt.Action) {
			bb = append(bb, tt)
		}
	}
	return bb
}

func (s *_baseSuite) newTid() string {
	s.tidCounter++
	return fmt.Sprintf("tid_%d", s.tidCounter)
}

func (s *_baseSuite) initServers(txnType string) {
	s.lessee = "test_lessee"
	s.notify = "http://dtxtest/notify"
	s.commitBranch = define.BranchTypeConfirm
	s.rollbackBranch = define.BranchTypeCancel

	if txnType == define.TxnTypeSaga {
		s.commitBranch = define.BranchTypeCommit
		s.rollbackBranch = define.BranchTypeCompensation
	}

	duration := time.Millisecond * 10
	s.serverNames = []string{"service0", "service1", "service2"}
	s.servers = newRmMock(map[string]*rmMockHandler{
		"http://service0": {
			name:           s.serverNames[0],
			branch:         s.commitBranch,
			requestTimeout: duration,
		},
		"http://service0/rollback": {
			name:           s.serverNames[0],
			branch:         s.rollbackBranch,
			requestTimeout: duration,
		},
		"http://service1": {
			name:           s.serverNames[1],
			branch:         s.commitBranch,
			requestTimeout: duration,
		},
		"http://service1/rollback": {
			name:           s.serverNames[1],
			branch:         s.rollbackBranch,
			requestTimeout: duration,
		},
		"http://service2": {
			name:           s.serverNames[2],
			branch:         s.commitBranch,
			requestTimeout: duration,
		},
		"http://service2/rollback": {
			name:           s.serverNames[2],
			branch:         s.rollbackBranch,
			requestTimeout: duration,
		},
	})
}

func (s *_baseSuite) checkStorage(store model.ModelStorage, sm *model.Txn) {
	get, err := store.GetByGtid(context.Background(), sm.Gtid)
	s.Nil(err)
	s.NotEqual(0, get.Id)
	s.Equal(sm.Gtid, get.Gtid)
	s.Equal(sm.State, get.State)
	s.Equal(sm.UpdatedTime.Unix(), get.UpdatedTime.Unix())
	s.Equal(sm.CreatedTime.Unix(), get.CreatedTime.Unix())
	s.NotEmpty(sm.Lessee)
	s.Equal(sm.CallType, get.CallType)
	s.Equal(sm.NotifyAction, get.NotifyAction)
	s.Equal(sm.NotifyTimeout, get.NotifyTimeout)
	s.Equal(sm.NotifyRetry, get.NotifyRetry)
	s.Equal(sm.ParallelExecution, get.ParallelExecution)
	s.Equal(sm.Business, get.Business)

	// subs
	s.Equal(len(sm.Branches), len(get.Branches))
	for i := 0; i < len(sm.Branches); i++ {
		exp := sm.Branches[i]
		act := get.Branches[i]
		s.NotEqual(0, act.Id)
		s.Equal(exp.Gtid, act.Gtid)
		s.Equal(exp.Bid, act.Bid)
		s.Equal(exp.BranchType, act.BranchType)
		s.Equal(exp.Action, act.Action)
		s.Equal(exp.Payload, act.Payload)
		s.Equal(exp.Timeout, act.Timeout)
		s.Equal(exp.State, act.State)
		s.Equal(exp.UpdatedTime.Unix(), act.UpdatedTime.Unix())
		s.Equal(exp.CreatedTime.Unix(), act.CreatedTime.Unix())
		s.Equal(exp.Retry.MaxRetry, act.Retry.MaxRetry)
	}
}
