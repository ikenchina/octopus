package tcc

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
	"github.com/ikenchina/octopus/tc/app/executor"
	"github.com/ikenchina/octopus/tc/app/model"
	"github.com/ikenchina/octopus/tc/config"
)

type TccService struct {
	cfg      Config
	wait     sync.WaitGroup
	executor *executor.TccExecutor
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

func NewTccService(cfg Config) (*TccService, error) {
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
		cfg:      cfg,
		executor: executor,
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
		notify.Done(err, "")
		return
	}
	switch code {
	case 200:
		notify.Done(nil, string(body))
	default:
		notify.Done(fmt.Errorf("http code is %d", code), "")
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
				switch sn.BranchType {
				case define.BranchTypeConfirm:
					ss.httpAction(sn, http.MethodPut)
				case define.BranchTypeCancel:
					ss.httpAction(sn, http.MethodDelete)
				}
			}
		}()
	}
}

// RESTful APIs

func (ss *TccService) parse(c *gin.Context) (*model.Txn, error) {
	ss.wait.Add(1)
	defer ss.wait.Done()

	body, err := c.GetRawData()
	if err != nil {
		return nil, err
	}

	request := &define.TccRequest{
		DcId:   ss.cfg.DataCenterId,
		NodeId: ss.cfg.NodeId,
		Lessee: ss.cfg.Lessee,
	}

	err = json.Unmarshal(body, request)
	if err != nil {
		return nil, err
	}
	err = validate(request)
	if err != nil {
		return nil, err
	}
	model := convertToModel(request)
	return model, nil
}

func (ss *TccService) closed() bool {
	return atomic.LoadInt32(&ss.close) == 1
}

func (ss *TccService) Prepare(c *gin.Context) {
	if ss.closed() {
		c.JSON(http.StatusServiceUnavailable, "")
		return
	}
	ss.prepare(c)
}

func (ss *TccService) prepare(c *gin.Context) {
	ss.wait.Add(1)
	defer ss.wait.Done()

	task, err := ss.parse(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, &define.TccResponse{
			Msg: fmt.Sprintf("ERROR : %v", err),
		})
		return
	}

	ctx := c.Request.Context()
	future := ss.executor.Prepare(ctx, task)
	resp := &define.TccResponse{
		Gtid:  task.Gtid,
		State: define.TxnStatePrepared,
	}

	err = future.GetError()

	parseFromModel(resp, future.Txn)
	if err != nil {
		resp.Msg = fmt.Sprintf("ERROR : %v", err)
	}

	c.JSON(ss.toStatusCode(err), resp)
}

func (ss *TccService) Register(c *gin.Context) {
	if ss.closed() {
		c.JSON(http.StatusServiceUnavailable, "")
		return
	}
	ss.register(c)
}

func (ss *TccService) register(c *gin.Context) {
	ss.wait.Add(1)
	defer ss.wait.Done()

	task, err := ss.parse(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, &define.TccResponse{
			Msg: fmt.Sprintf("ERROR : %v", err),
		})
		return
	}

	ctx := c.Request.Context()
	future := ss.executor.Register(ctx, task)
	resp := &define.TccResponse{
		Gtid:  task.Gtid,
		State: define.TxnStatePrepared,
	}

	err = future.GetError()
	parseFromModel(resp, future.Txn)
	if err != nil {
		resp.Msg = fmt.Sprintf("ERROR : %v", err)
	}
	c.JSON(ss.toStatusCode(err), resp)
}

func (ss *TccService) Confirm(c *gin.Context) {
	if ss.closed() {
		c.JSON(http.StatusServiceUnavailable, "")
		return
	}
	ss.confirm(c)
}

func (ss *TccService) confirm(c *gin.Context) {
	gtid := c.Param("gtid")
	future := ss.executor.Commit(c.Request.Context(), &model.Txn{
		Gtid: gtid,
	})
	err := future.GetError()

	resp := &define.TccResponse{}
	parseFromModel(resp, future.Txn)
	if err != nil {
		resp.Msg = fmt.Sprintf("ERROR : %v", err)
	}

	c.JSON(ss.toStatusCode(err), resp)
}

func (ss *TccService) Cancel(c *gin.Context) {
	if ss.closed() {
		c.JSON(http.StatusServiceUnavailable, "")
		return
	}
	ss.cancel(c)
}

func (ss *TccService) cancel(c *gin.Context) {
	gtid := c.Param("gtid")
	future := ss.executor.Rollback(c.Request.Context(), &model.Txn{
		Gtid: gtid,
	})

	err := future.GetError()

	resp := &define.TccResponse{}
	parseFromModel(resp, future.Txn)
	if err != nil {
		resp.Msg = fmt.Sprintf("ERROR : %v", err)
	}
	c.JSON(ss.toStatusCode(err), resp)
}

func (ss *TccService) Get(c *gin.Context) {
	if ss.closed() {
		c.JSON(http.StatusServiceUnavailable, "")
		return
	}
	ss.get(c)
}

func (ss *TccService) get(c *gin.Context) {
	gtid := c.Param("gtid")
	txn, err := ss.executor.Get(c.Request.Context(), gtid)

	resp := &define.TccResponse{
		Gtid: gtid,
	}
	if err == nil {
		parseFromModel(resp, txn)
	} else {
		resp.Msg = fmt.Sprintf("ERROR : %v", err)
	}
	c.JSON(ss.toStatusCode(err), resp)
}

func (ss *TccService) toStatusCode(err error) int {
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
	case executor.ErrInProgress:
		return http.StatusOK
	case executor.ErrExecutorClosed:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}
