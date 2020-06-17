package saga

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	executor "github.com/ikenchina/octopus/app/executor"
	"github.com/ikenchina/octopus/app/model"
	"github.com/ikenchina/octopus/common/errorutil"
	shttp "github.com/ikenchina/octopus/common/http"
	logutil "github.com/ikenchina/octopus/common/log"
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
	Driver              string
	Dsn                 string
	Timeout             time.Duration
	MaxConcurrentTask   int
	Lessee              string
	MaxConcurrentBranch int
}

func NewSagaService(cfg Config) (*SagaService, error) {
	store, err := model.NewModelStorage(cfg.Driver, cfg.Dsn, cfg.Timeout, cfg.Lessee)
	if err != nil {
		return nil, err
	}

	executor, err := executor.NewSagaExecutor(store, cfg.MaxConcurrentTask, cfg.Lessee)
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
	code, body, err := shttp.Send(notify.Ctx, ss.cfg.DataCenterId, ss.cfg.NodeId, model.TxnTypeSaga,
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
		max = 1
	}
	for i := 0; i < max; i++ {
		ss.wait.Add(1)
		go func() {
			defer errorutil.Recovery()
			defer ss.wait.Done()
			for sn := range ss.executor.NotifyChan() {
				if sn.BranchType == model.BranchTypeCommit {
					ss.httpAction(sn, http.MethodPut)
				} else if sn.BranchType == model.BranchTypeCompensation {
					ss.httpAction(sn, http.MethodDelete)
				} else {
					ss.httpNotifyAction(sn)
				}
			}
		}()
	}
}

func (ss *SagaService) httpNotifyAction(sn *executor.ActionNotify) {
	sr := SagaResponse{}
	sr.ParseFromModel(sn.Txn())
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
	request := &SagaRequest{
		DcId:   ss.cfg.DataCenterId,
		NodeId: ss.cfg.NodeId,
		Lessee: ss.cfg.Lessee,
	}
	err = json.Unmarshal(body, request)
	if err != nil {
		return nil, err
	}
	err = request.Validate()
	if err != nil {
		return nil, err
	}
	sagaModel := request.ConvertToModel()
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
		c.JSON(http.StatusBadRequest, &SagaResponse{
			Msg: err.Error(),
		})
		return
	}

	ctx := c.Request.Context()
	if saga.CallType == model.TxnCallTypeAsync {
		ctx = context.Background()
	}

	future := ss.executor.Commit(ctx, saga)
	resp := &SagaResponse{
		Gtid:  saga.Gtid,
		State: model.TxnStatePrepared,
	}

	if saga.CallType == model.TxnCallTypeSync {
		err = future.GetError()
	} else {
		select {
		case err = <-future.Get():
		default:
		}
	}
	resp.ParseFromModel(future.Txn)
	if err != nil {
		resp.Msg = err.Error()
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
	resp := &SagaResponse{
		Gtid: gtid,
	}
	if err != nil {
		resp.Msg = err.Error()
		c.JSON(http.StatusOK, resp)
		return
	}
	resp.ParseFromModel(saga)
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
