package saga

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/gin-gonic/gin"

	"github.com/ikenchina/octopus/common/errorutil"
	shttp "github.com/ikenchina/octopus/common/http"
	logutil "github.com/ikenchina/octopus/common/log"
	"github.com/ikenchina/octopus/define"
	executor "github.com/ikenchina/octopus/tc/app/executor"
	"github.com/ikenchina/octopus/tc/app/model"
	"github.com/ikenchina/octopus/tc/config"
)

type SagaService struct {
	cfg      Config
	executor *executor.SagaExecutor
	wait     sync.WaitGroup
	close    int32
}

type Config struct {
	NodeId              int
	DataCenterId        int
	Store               config.StorageConfig
	MaxConcurrentTask   int
	Lessee              string
	MaxConcurrentBranch int
}

func NewSagaService(cfg Config) (*SagaService, error) {
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
		executor: executor,
	}, nil
}

func (ss *SagaService) Start() error {
	logutil.Logger(context.Background()).Info("start saga service.")
	err := ss.executor.Start()
	if err != nil {
		return err
	}
	go ss.notifyHandle()
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
		notify.Done(err, "")
		return
	}

	if code >= 200 && code < 300 {
		notify.Done(nil, body)
	} else {
		notify.Done(fmt.Errorf("http code is %d", code), "")
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
				if sn.BranchType == define.BranchTypeCommit {
					ss.httpAction(sn, http.MethodPost)
				} else if sn.BranchType == define.BranchTypeCompensation {
					ss.httpAction(sn, http.MethodDelete)
				} else {
					ss.httpNotifyAction(sn)
				}
			}
		}()
	}
}

func (ss *SagaService) httpNotifyAction(sn *executor.ActionNotify) {
	sr := define.SagaResponse{}
	parseFromModel(&sr, sn.Txn())
	payload, _ := json.Marshal(sr)
	sn.Payload = string(payload)
	ss.httpAction(sn, http.MethodPost)
}

// RESTful APIs

func (ss *SagaService) parseSaga(c *gin.Context) (*model.Txn, error) {
	ss.wait.Add(1)
	defer ss.wait.Done()

	body, err := c.GetRawData()
	if err != nil {
		return nil, err
	}
	request := &define.SagaRequest{
		DcId:   ss.cfg.DataCenterId,
		NodeId: ss.cfg.NodeId,
		Lessee: ss.cfg.Lessee,
	}
	err = json.Unmarshal(body, request)
	if err != nil {
		return nil, err
	}
	err = Validate(request)
	if err != nil {
		return nil, err
	}
	sagaModel := convertToModel(request)
	return sagaModel, nil
}

func (ss *SagaService) closed() bool {
	return atomic.LoadInt32(&ss.close) == 1
}

func (ss *SagaService) Commit(c *gin.Context) {
	if ss.closed() {
		c.JSON(http.StatusServiceUnavailable, "")
		return
	}
	ss.commit(c)
}

func (ss *SagaService) commit(c *gin.Context) {
	ss.wait.Add(1)
	defer ss.wait.Done()

	saga, err := ss.parseSaga(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, &define.SagaResponse{
			Msg: fmt.Sprintf("ERROR : %v", err),
		})
		return
	}

	ctx := c.Request.Context()
	if saga.CallType == define.TxnCallTypeAsync {
		ctx = context.Background()
	}

	future := ss.executor.Commit(ctx, saga)
	resp := &define.SagaResponse{
		Gtid:  saga.Gtid,
		State: define.TxnStatePrepared,
	}

	if saga.CallType == define.TxnCallTypeSync {
		err = future.GetError()
	} else {
		select {
		case err = <-future.Get():
		default:
		}
	}
	parseFromModel(resp, future.Txn)
	if err != nil {
		resp.Msg = fmt.Sprintf("ERROR : %v", err)
	}

	c.JSON(ss.toStatusCode(err), resp)
}

func (ss *SagaService) Get(c *gin.Context) {
	if ss.closed() {
		c.JSON(http.StatusServiceUnavailable, "")
		return
	}
	ss.get(c)
}

func (ss *SagaService) get(c *gin.Context) {
	ss.wait.Add(1)
	defer ss.wait.Done()

	gtid := c.Param("gtid")
	saga, err := ss.executor.Get(context.Background(), gtid)
	resp := &define.SagaResponse{
		Gtid: gtid,
	}
	if err != nil {
		resp.Msg = fmt.Sprintf("ERROR : %v", err)
		c.JSON(http.StatusOK, resp)
		return
	}
	parseFromModel(resp, saga)
	c.JSON(ss.toStatusCode(err), resp)
}

func (ss *SagaService) toStatusCode(err error) int {
	if err == nil {
		return http.StatusOK
	}
	switch err {
	case executor.ErrTimeout:
		return http.StatusRequestTimeout
	case executor.ErrTaskAlreadyExist:
		return http.StatusConflict
	case executor.ErrNotExists:
		return http.StatusNotFound
	case executor.ErrExecutorClosed:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}
