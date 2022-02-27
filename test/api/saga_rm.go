package test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	saga_client "github.com/ikenchina/octopus/client/saga"
	"github.com/ikenchina/octopus/common/errorutil"
	"github.com/ikenchina/octopus/define"
)

type sagaRmMock struct {
	*rmMock
	notify     map[string]*define.SagaResponse
	requests   map[string]*define.SagaRequest
	notifyChan chan *define.SagaResponse
	cli        saga_client.Client
}

func newSagaRm(server int, listen string) *sagaRmMock {
	rm := &sagaRmMock{
		notify:     make(map[string]*define.SagaResponse),
		requests:   make(map[string]*define.SagaRequest),
		notifyChan: make(chan *define.SagaResponse, 10),
	}
	rm.rmMock = newRm("/dtx/saga", server, listen, rm)
	rm.cli.TcServer = rm.tcDomain
	return rm
}

func (rm *sagaRmMock) notifyHandler() func(c *gin.Context) {
	return func(c *gin.Context) {
		defer errorutil.Recovery(func(i interface{}) {
			c.JSON(500, i)
		})
		body, err := c.GetRawData()
		errorutil.PanicIfError(err)

		rs := &define.SagaResponse{}
		err = json.Unmarshal(body, rs)
		errorutil.PanicIfError(err)
		gtid := c.Param("gtid")
		rm.notify[gtid] = rs
		rm.notifyChan <- rs
	}
}

func (rm *sagaRmMock) commitHandler(index int) func(c *gin.Context) {
	return func(c *gin.Context) {
	}
}

func (rm *sagaRmMock) actionHandler(index int) func(c *gin.Context) {
	return func(c *gin.Context) {
		defer errorutil.Recovery(func(i interface{}) {
			c.JSON(500, i)
		})
		gtid := c.Param("gtid")
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
		gtid := c.Param("gtid")
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

func (rm *sagaRmMock) newSagaRequest() (*define.SagaRequest, error) {
	gtid, err := rm.cli.NewGtid(context.Background())
	errorutil.PanicIfError(err)
	req := &define.SagaRequest{
		Gtid:       gtid,
		Business:   "test",
		ExpireTime: time.Now().Add(1 * time.Hour),
		Notify: &define.SagaNotify{
			Action:  rm.rmDomain + rm.basePath,
			Timeout: time.Millisecond * 200,
			Retry:   time.Millisecond * 200,
		},
	}
	for i, svr := range rm.servers {
		bid := fmt.Sprintf("%d", i+1)
		req.Branches = append(req.Branches, define.SagaBranch{
			BranchId: i + 1,
			Payload:  bid,
			Commit: define.SagaBranchCommit{
				Action:  fmt.Sprintf("%s/%s/%d", svr.commitUrl, gtid, i+1),
				Timeout: time.Millisecond * 200,
				Retry: define.SagaRetry{
					MaxRetry: 3,
					Constant: &define.RetryStrategyConstant{
						Duration: time.Millisecond * 200,
					},
				},
			},
			Compensation: define.SagaBranchCompensation{
				Action:  fmt.Sprintf("%s/%s/%d", svr.commitUrl, gtid, i+1),
				Timeout: time.Millisecond * 200,
				Retry:   time.Millisecond * 200,
			},
		})
	}

	rm.requests[gtid] = req
	return req, nil
}
