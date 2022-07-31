package saga

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ikenchina/octopus/define"
	executor "github.com/ikenchina/octopus/tc/app/executor"
	"github.com/ikenchina/octopus/tc/app/model"
)

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
	return ss.convertToModel(request)
}

func (ss *SagaService) OpTxn(c *gin.Context) {
	// @todo
}

func (ss *SagaService) HttpCommit(c *gin.Context) {
	code := http.StatusOK
	defer requestTimer.Timer()("http", "Commit", http.StatusText(code))

	if ss.closed() {
		code = http.StatusServiceUnavailable
		c.JSON(code, "")
		return
	}

	ss.wait.Add(1)
	defer ss.wait.Done()

	saga, err := ss.parseSaga(c)
	if err != nil {
		code = http.StatusBadRequest
		c.JSON(code, &define.SagaResponse{
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
	ss.parseFromModel(resp, future.Txn)
	if err != nil {
		resp.Msg = fmt.Sprintf("ERROR : %v", err)
	}

	code = ss.toHttpStatusCode(err)
	c.JSON(code, resp)
}

func (ss *SagaService) HttpGet(c *gin.Context) {
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
	saga, err := ss.executor.Get(context.Background(), gtid)
	resp := &define.SagaResponse{
		Gtid: gtid,
	}
	if err != nil {
		resp.Msg = fmt.Sprintf("ERROR : %v", err)
		code = ss.toHttpStatusCode(err)
		c.JSON(code, resp)
		return
	}

	ss.parseFromModel(resp, saga)
	code = ss.toHttpStatusCode(err)
	c.JSON(code, resp)
}

func (ss *SagaService) toHttpStatusCode(err error) int {
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
