package saga

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/ikenchina/octopus/common/errorutil"
	sgrpc "github.com/ikenchina/octopus/common/grpc"
	shttp "github.com/ikenchina/octopus/common/http"
	"github.com/ikenchina/octopus/common/idgenerator"
	logutil "github.com/ikenchina/octopus/common/log"
	"github.com/ikenchina/octopus/common/util"
	"github.com/ikenchina/octopus/define"
	tc_rpc "github.com/ikenchina/octopus/define/proto/saga/pb"
	executor "github.com/ikenchina/octopus/tc/app/executor"
	"github.com/ikenchina/octopus/tc/app/model"
	"github.com/ikenchina/octopus/tc/config"
)

type SagaService struct {
	tc_rpc.UnimplementedTcServer
	cfg         Config
	executor    *executor.SagaExecutor
	wait        sync.WaitGroup
	close       int32
	idGenerator idgenerator.IdGenerator
}

type Config struct {
	NodeId              int
	DataCenterId        int
	Store               config.StorageConfig
	MaxConcurrentTask   int
	Lessee              string
	MaxConcurrentBranch int
}

func NewSagaService(cfg Config, idGenerator idgenerator.IdGenerator) (*SagaService, error) {
	store, err := model.NewModelStorage(cfg.Store.Driver, cfg.Store.Dsn,
		cfg.Store.Timeout, cfg.Store.MaxConnections, cfg.Store.MaxIdleConnections, cfg.Lessee)
	if err != nil {
		return nil, err
	}

	executor, err := executor.NewSagaExecutor(executor.Config{
		Store:                     store,
		MaxConcurrency:            cfg.MaxConcurrentTask,
		Lessee:                    cfg.Lessee,
		LeaseExpiredLimit:         cfg.Store.LeaseExpiredLimit,
		CleanExpired:              cfg.Store.CleanExpired,
		CleanLimit:                cfg.Store.CleanLimit,
		CheckLeaseExpiredDuration: cfg.Store.CheckLeaseExpiredDuration,
		CheckExpiredDuration:      cfg.Store.CheckExpiredDuration,
	})
	if err != nil {
		return nil, err
	}

	return &SagaService{cfg: cfg,
		executor:    executor,
		idGenerator: idGenerator,
	}, nil
}

func (ss *SagaService) Start() error {
	logutil.Logger(context.Background()).Info("start saga service.")
	err := ss.executor.Start()
	if err != nil {
		return err
	}
	go ss.notifyHandle()
	atomic.StoreInt32(&ss.close, 0)
	return nil
}

func (ss *SagaService) Stop() error {
	logutil.Logger(context.Background()).Info("stop saga service.")
	if atomic.CompareAndSwapInt32(&ss.close, 0, 1) {
		err := ss.executor.Stop()
		if err != nil {
			return err
		}
		ss.wait.Wait()
	}
	return nil
}

func (ss *SagaService) httpAction(notify *executor.ActionNotify, method string) {
	code, body, err := shttp.Send(notify.Ctx, ss.cfg.DataCenterId, ss.cfg.NodeId, define.TxnTypeSaga,
		notify.GID(), method, notify.Action, notify.Payload)
	if err != nil {
		notify.Done(err, nil)
		return
	}

	if code >= 200 && code < 300 {
		notify.Done(nil, body)
	} else {
		notify.Done(fmt.Errorf("http code is %d", code), nil)
	}
}

func (ss *SagaService) notifyHandle() {
	max := ss.cfg.MaxConcurrentBranch
	if max <= 0 {
		max = 10
	}
	for i := 0; i < max; i++ {
		ss.wait.Add(1)
		go func() {
			defer errorutil.Recovery()
			defer ss.wait.Done()
			for sn := range ss.executor.NotifyChan() {
				sch, domain, method := util.ExtractAction(sn.Action)
				switch sch {
				case "http", "https":
					ss.httpHandle(sn)
				case "grpc":
					ss.grpcHandle(sn, domain, method)
				default:
					sn.Done(ErrInvalidAction, nil)
				}
			}
		}()
	}
}

func (ss *SagaService) grpcHandle(sn *executor.ActionNotify, domain, method string) {
	resp, err := sgrpc.Invoke(sn.Ctx, domain, sn.Txn().Gtid, sn.BranchID, method, sn.Payload)
	if err != nil {
		sn.Done(err, nil)
		return
	}
	sn.Done(nil, (resp))
}

func (ss *SagaService) httpHandle(sn *executor.ActionNotify) {
	if sn.BranchType == define.BranchTypeCommit {
		ss.httpAction(sn, http.MethodPost)
	} else if sn.BranchType == define.BranchTypeCompensation {
		ss.httpAction(sn, http.MethodDelete)
	} else {
		ss.httpNotifyAction(sn)
	}
}

func (ss *SagaService) httpNotifyAction(sn *executor.ActionNotify) {
	sr := define.SagaResponse{}
	ss.parseFromModel(&sr, sn.Txn())
	payload, _ := json.Marshal(sr)
	sn.Payload = payload
	ss.httpAction(sn, http.MethodPost)
}

func (ss *SagaService) closed() bool {
	return atomic.LoadInt32(&ss.close) == 1
}
