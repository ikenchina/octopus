package test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ikenchina/octopus/common/errorutil"
	shttp "github.com/ikenchina/octopus/common/http"
	"github.com/ikenchina/octopus/service/tcc"
)

type tccRmMock struct {
	*rmMock
	notify     map[string]*tcc.TccResponse
	requests   map[string]*tcc.TccRequest
	notifyChan chan *tcc.TccResponse
}

func newTccRm(server int, listen string) *tccRmMock {
	rm := &tccRmMock{
		notify:     make(map[string]*tcc.TccResponse),
		requests:   make(map[string]*tcc.TccRequest),
		notifyChan: make(chan *tcc.TccResponse, 10),
	}
	rm.rmMock = newRm("/dtx/tcc", server, listen, rm)
	return rm
}

func (rm *tccRmMock) notifyHandler() func(c *gin.Context) {
	return func(c *gin.Context) {
		defer errorutil.Recovery(func(i interface{}) {
			c.JSON(500, i)
		})
		body, err := c.GetRawData()
		errorutil.PanicIfError(err)

		rs := &tcc.TccResponse{}
		err = json.Unmarshal(body, rs)
		errorutil.PanicIfError(err)
		gtid := c.Param("id")
		rm.notify[gtid] = rs
		rm.notifyChan <- rs
	}
}

func (rm *tccRmMock) commitHandler(index int) func(c *gin.Context) {
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

func (rm *tccRmMock) rollbackHandler(index int) func(c *gin.Context) {
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

func (rm *tccRmMock) newTccRequest() (*tcc.TccRequest, error) {
	data := make(map[string]string)
	code, err := shttp.Get(context.Background(), "", rm.tcDomain+rm.basePath+"/gtid", &data)
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("status code : %d", code)
	}
	errorutil.PanicIfError(err)
	gtid := data["gtid"]

	req := &tcc.TccRequest{
		Gtid:       gtid,
		Business:   "test",
		ExpireTime: time.Now().Add(1 * time.Hour),
	}
	for i, svr := range rm.servers {
		bid := fmt.Sprintf("%d", i)
		req.Branches = append(req.Branches, tcc.TccBranch{
			BranchId:      bid,
			Payload:       bid,
			ActionConfirm: svr.commitUrl + "/" + gtid,
			ActionCancel:  svr.rollbackUrl + "/" + gtid,
			Timeout:       time.Millisecond * 200,
			Retry:         time.Millisecond * 200,
		})
	}

	rm.requests[gtid] = req
	return req, nil
}
