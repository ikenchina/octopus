package tcc

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
	tcc_pb "github.com/ikenchina/octopus/define/proto/tcc/pb"
	"github.com/ikenchina/octopus/tc/app/executor"
	"github.com/ikenchina/octopus/tc/app/model"
	"github.com/ikenchina/octopus/tc/config"
)

type TccService struct {
	tcc_pb.UnimplementedTcServer
	cfg         Config
	wait        sync.WaitGroup
	executor    *executor.TccExecutor
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

func NewTccService(cfg Config, idGenerator idgenerator.IdGenerator) (*TccService, error) {
	store, err := model.NewModelStorage(cfg.Store.Driver, cfg.Store.Dsn, cfg.Store.Timeout,
		cfg.Store.MaxConnections, cfg.Store.MaxIdleConnections, cfg.Lessee)
	if err != nil {
		return nil, err
	}

	executor, err := executor.NewTccExecutor(executor.Config{
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

	tcc := &TccService{
		cfg:         cfg,
		executor:    executor,
		idGenerator: idGenerator,
	}
	return tcc, nil
}

func (ss *TccService) Start() error {
	logutil.Logger(context.Background()).Info("start tcc service.")
	err := ss.executor.Start()
	if err != nil {
		return err
	}
	go ss.notifyHandle()
	return nil
}

func (ss *TccService) Stop() error {
	if atomic.CompareAndSwapInt32(&ss.close, 0, 1) {
		err := ss.executor.Stop()
		if err != nil {
			return err
		}
		ss.wait.Wait()
	}
	return nil
}

func (ss *TccService) httpAction(notify *executor.ActionNotify, method string) {
	code, body, err := shttp.Send(notify.Ctx, ss.cfg.DataCenterId, ss.cfg.NodeId, define.TxnTypeTcc,
		notify.GID(), method, notify.Action, notify.Payload)
	if err != nil {
		notify.Done(err, nil)
		return
	}
	switch code {
	case 200:
		notify.Done(nil, body)
	default:
		notify.Done(fmt.Errorf("http code is %d", code), nil)
	}
}

func (ss *TccService) notifyHandle() {
	max := ss.cfg.MaxConcurrentBranch
	if max <= 0 {
		max = 1
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

func (ss *TccService) grpcHandle(sn *executor.ActionNotify, domain, method string) {
	resp, err := sgrpc.Invoke(sn.Ctx, domain, sn.GID(), sn.BranchID, method, sn.Payload)
	if err != nil {
		sn.Done(err, nil)
		return
	}
	sn.Done(nil, (resp))
}

func (ss *TccService) httpHandle(sn *executor.ActionNotify) {
	if sn.BranchType == define.BranchTypeCommit {
		ss.httpAction(sn, http.MethodPost)
	} else if sn.BranchType == define.BranchTypeCompensation {
		ss.httpAction(sn, http.MethodDelete)
	} else {
		ss.httpNotifyAction(sn)
	}
}

func (ss *TccService) httpNotifyAction(sn *executor.ActionNotify) {
	sr := define.TccResponse{}
	ss.parseFromModel(&sr, sn.Txn())
	payload, _ := json.Marshal(sr)
	sn.Payload = (payload)
	ss.httpAction(sn, http.MethodPost)
}

func (ss *TccService) closed() bool {
	return atomic.LoadInt32(&ss.close) == 1
}
