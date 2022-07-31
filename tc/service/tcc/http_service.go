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
	code := http.StatusOK
	defer requestTimer.Timer()("http", "Prepare", http.StatusText(code))

	if ss.closed() {
		code = http.StatusServiceUnavailable
		c.JSON(code, "")
		return
	}

	ss.wait.Add(1)
	defer ss.wait.Done()

	task, err := ss.parse(c)
	if err != nil || task.ExpireTime.Before(time.Now()) {
		code = http.StatusBadRequest
		c.JSON(code, &define.TccResponse{
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

	code = ss.toStatusCode(err)
	c.JSON(code, resp)
}

func (ss *TccService) OpTxn(c *gin.Context) {
	// @todo
}

func (ss *TccService) HttpRegister(c *gin.Context) {
	code := http.StatusOK
	defer requestTimer.Timer()("http", "Register", http.StatusText(code))

	if ss.closed() {
		code = http.StatusServiceUnavailable
		c.JSON(code, "")
		return
	}

	ss.wait.Add(1)
	defer ss.wait.Done()

	task, err := ss.parse(c)
	if err != nil {
		code = http.StatusBadRequest
		c.JSON(code, &define.TccResponse{
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

	code = ss.toStatusCode(err)
	c.JSON(code, resp)
}

func (ss *TccService) HttpConfirm(c *gin.Context) {
	code := http.StatusOK
	defer requestTimer.Timer()("http", "Confirm", http.StatusText(code))

	if ss.closed() {
		code = http.StatusServiceUnavailable
		c.JSON(code, "")
		return
	}

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

	code = ss.toStatusCode(err)
	c.JSON(code, resp)
}

func (ss *TccService) HttpCancel(c *gin.Context) {
	code := http.StatusOK
	defer requestTimer.Timer()("http", "Cancel", http.StatusText(code))

	if ss.closed() {
		code = http.StatusServiceUnavailable
		c.JSON(code, "")
		return
	}

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

	code = ss.toStatusCode(err)
	c.JSON(code, resp)
}

func (ss *TccService) HttpGet(c *gin.Context) {
	code := http.StatusOK
	defer requestTimer.Timer()("http", "Get", http.StatusText(code))

	if ss.closed() {
		code = http.StatusServiceUnavailable
		c.JSON(code, "")
		return
	}

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

	code = ss.toStatusCode(err)
	c.JSON(code, resp)
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
