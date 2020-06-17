package test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ikenchina/octopus/common/errorutil"
	shttp "github.com/ikenchina/octopus/common/http"
	"github.com/ikenchina/octopus/service/saga"
)

type sagaRmMock struct {
	*rmMock
	notify     map[string]*saga.SagaResponse
	requests   map[string]*saga.SagaRequest
	notifyChan chan *saga.SagaResponse
}

func newSagaRm(server int, listen string) *sagaRmMock {
	rm := &sagaRmMock{
		notify:     make(map[string]*saga.SagaResponse),
		requests:   make(map[string]*saga.SagaRequest),
		notifyChan: make(chan *saga.SagaResponse, 10),
	}
	rm.rmMock = newRm("/dtx/saga", server, listen, rm)
	return rm
}

func (rm *sagaRmMock) notifyHandler() func(c *gin.Context) {
	return func(c *gin.Context) {
		defer errorutil.Recovery(func(i interface{}) {
			c.JSON(500, i)
		})
		body, err := c.GetRawData()
		errorutil.PanicIfError(err)

		rs := &saga.SagaResponse{}
		err = json.Unmarshal(body, rs)
		errorutil.PanicIfError(err)
		gtid := c.Param("id")
		rm.notify[gtid] = rs
		rm.notifyChan <- rs
	}
}

func (rm *sagaRmMock) commitHandler(index int) func(c *gin.Context) {
	return func(c *gin.Context) {
		defer errorutil.Recovery(func(i interface{}) {
			c.JSON(500, i)
		})
		gtid := c.Param("id")
		svr := rm.servers[index]
		data, ok := svr.data[gtid]
		if !ok {
			svr.data[gtid] = &rmServerData{}
			data = svr.data[gtid]
		}
		// body, err := c.GetRawData()
		// if err != nil {
		// 	c.JSON(http.StatusBadRequest, "")
		// }
		data.commit = true
		time.Sleep(svr.timeout)
		svr.commitChan <- gtid
	}
}

func (rm *sagaRmMock) rollbackHandler(index int) func(c *gin.Context) {
	return func(c *gin.Context) {
		defer errorutil.Recovery(func(i interface{}) {
			c.JSON(500, i)
		})
		gtid := c.Param("id")
		svr := rm.servers[index]
		data, ok := svr.data[gtid]
		if !ok {
			svr.data[gtid] = &rmServerData{}
			data = svr.data[gtid]
		}
		// body, err := c.GetRawData()
		// if err != nil {
		// 	c.JSON(http.StatusBadRequest, "")
		// }
		data.rollback = true
		time.Sleep(svr.rollbackTimeout)
		svr.rollbackChan <- gtid
	}
}

func (rm *sagaRmMock) newSagaRequest() (*saga.SagaRequest, error) {
	data := make(map[string]string)
	code, err := shttp.Get(context.Background(), "", rm.tcDomain+rm.basePath+"/gtid", &data)
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("status code : %d", code)
	}
	errorutil.PanicIfError(err)
	gtid := data["gtid"]

	req := &saga.SagaRequest{
		Gtid:       gtid,
		Business:   "test",
		ExpireTime: time.Now().Add(1 * time.Hour),
		Notify: &saga.SagaNotify{
			Action:  rm.rmDomain + rm.basePath,
			Timeout: time.Millisecond * 200,
			Retry:   time.Millisecond * 200,
		},
	}
	for i, svr := range rm.servers {
		bid := fmt.Sprintf("%d", i)
		req.Branches = append(req.Branches, saga.SagaBranch{
			BranchId: bid,
			Payload:  bid,
			Commit: saga.SagaBranchCommit{
				Action:  svr.commitUrl + "/" + gtid,
				Timeout: time.Millisecond * 200,
				Retry: saga.SagaRetry{
					MaxRetry: 3,
					Constant: &saga.RetryStrategyConstant{
						Duration: time.Millisecond * 200,
					},
				},
			},
			Compensation: saga.SagaBranchCompensation{
				Action:  svr.rollbackUrl + "/" + gtid,
				Timeout: time.Millisecond * 200,
				Retry:   time.Millisecond * 200,
			},
		})
	}

	rm.requests[gtid] = req
	return req, nil
}
