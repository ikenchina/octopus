package tcc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ikenchina/octopus/define"
	"github.com/ikenchina/octopus/tc/app/executor"
	"github.com/ikenchina/octopus/tc/app/model"
)

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
	return ss.convertToModel(request)
}

func (ss *TccService) HttpPrepare(c *gin.Context) {
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
	if err != nil || task.ExpireTime.Before(time.Now()) {
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
	ss.parseFromModel(resp, future.Txn)

	err = future.GetError()
	if err != nil {
		resp.Msg = fmt.Sprintf("ERROR : %v", err)
	}
	c.JSON(ss.toStatusCode(err), resp)
}

func (ss *TccService) HttpRegister(c *gin.Context) {
	if ss.closed() {
		c.JSON(http.StatusServiceUnavailable, "")
		return
	}
	ss.register(c)
}

func (ss *TccService) OpTxn(c *gin.Context) {
	// @todo
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
	ss.parseFromModel(resp, future.Txn)
	if err != nil {
		resp.Msg = fmt.Sprintf("ERROR : %v", err)
	}
	c.JSON(ss.toStatusCode(err), resp)
}

func (ss *TccService) HttpConfirm(c *gin.Context) {
	if ss.closed() {
		c.JSON(http.StatusServiceUnavailable, "")
		return
	}
	ss.confirm(c)
}

func (ss *TccService) confirm(c *gin.Context) {
	ss.wait.Add(1)
	defer ss.wait.Done()

	gtid := c.Param("gtid")
	future := ss.executor.Commit(c.Request.Context(), &model.Txn{
		Gtid: gtid,
	})
	err := future.GetError()

	resp := &define.TccResponse{}
	ss.parseFromModel(resp, future.Txn)
	if err != nil {
		resp.Msg = fmt.Sprintf("ERROR : %v", err)
	}

	c.JSON(ss.toStatusCode(err), resp)
}

func (ss *TccService) HttpCancel(c *gin.Context) {
	if ss.closed() {
		c.JSON(http.StatusServiceUnavailable, "")
		return
	}
	ss.cancel(c)
}

func (ss *TccService) cancel(c *gin.Context) {
	ss.wait.Add(1)
	defer ss.wait.Done()

	gtid := c.Param("gtid")
	future := ss.executor.Rollback(c.Request.Context(), &model.Txn{
		Gtid: gtid,
	})

	err := future.GetError()

	resp := &define.TccResponse{}
	ss.parseFromModel(resp, future.Txn)
	if err != nil {
		resp.Msg = fmt.Sprintf("ERROR : %v", err)
	}
	c.JSON(ss.toStatusCode(err), resp)
}

func (ss *TccService) HttpGet(c *gin.Context) {
	if ss.closed() {
		c.JSON(http.StatusServiceUnavailable, "")
		return
	}
	ss.get(c)
}

func (ss *TccService) get(c *gin.Context) {
	ss.wait.Add(1)
	defer ss.wait.Done()

	gtid := c.Param("gtid")
	txn, err := ss.executor.Get(c.Request.Context(), gtid)

	resp := &define.TccResponse{
		Gtid: gtid,
	}
	if err == nil {
		ss.parseFromModel(resp, txn)
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
	case executor.ErrTimeout, executor.ErrCancel:
		return http.StatusRequestTimeout
	case executor.ErrTaskAlreadyExist:
		return http.StatusConflict
	case executor.ErrNotExists:
		return http.StatusNotFound
	case executor.ErrInProgress:
		return http.StatusOK
	case executor.ErrStateIsAborted, executor.ErrStateIsCommitted:
		return http.StatusForbidden
	case executor.ErrExecutorClosed:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}
