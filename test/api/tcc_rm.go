package test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	tcc_cli "github.com/ikenchina/octopus/client/tcc"
	"github.com/ikenchina/octopus/common/errorutil"
	"github.com/ikenchina/octopus/define"
)

type tccRmMock struct {
	*rmMock
	notify     map[string]*define.TccResponse
	requests   map[string]*define.TccRequest
	notifyChan chan *define.TccResponse
}

func newTccRm(server int, listen string) *tccRmMock {
	rm := &tccRmMock{
		notify:     make(map[string]*define.TccResponse),
		requests:   make(map[string]*define.TccRequest),
		notifyChan: make(chan *define.TccResponse, 10),
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

		rs := &define.TccResponse{}
		err = json.Unmarshal(body, rs)
		errorutil.PanicIfError(err)
		gtid := c.Param("id")
		rm.notify[gtid] = rs
		rm.notifyChan <- rs
	}
}

func (rm *tccRmMock) actionHandler(index int) func(c *gin.Context) {
	return func(c *gin.Context) {
		defer errorutil.Recovery(func(i interface{}) {
			c.JSON(500, i)
		})
		gtid := c.Param("gtid")
		//bid := c.Param("branch_id")
		svr := rm.servers[index]
		data, ok := svr.data[gtid]
		if !ok {
			svr.data[gtid] = &rmServerData{}
			data = svr.data[gtid]
		}

		data.action = true
		time.Sleep(svr.actionTimeout)
		if svr.actionErr {
			c.JSON(http.StatusInternalServerError, nil)
		}
	}
}

func (rm *tccRmMock) commitHandler(index int) func(c *gin.Context) {
	return func(c *gin.Context) {
		defer errorutil.Recovery(func(i interface{}) {
			c.JSON(500, i)
		})
		gtid := c.Param("gtid")
		//bid := c.Param("branch_id")
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

func (rm *tccRmMock) newTccRequest() (*define.TccRequest, error) {
	cli := tcc_cli.Client{
		TcServer: rm.tcDomain,
	}
	gtid, err := cli.NewGtid(context.TODO())
	if err != nil {
		return nil, err
	}
	errorutil.PanicIfError(err)
	req := &define.TccRequest{
		Gtid:       gtid,
		Business:   "test",
		ExpireTime: time.Now().Add(1 * time.Hour),
	}
	for i, svr := range rm.servers {
		req.Branches = append(req.Branches, define.TccBranch{
			BranchId:      i + 1,
			Payload:       fmt.Sprintf("%d", i+1),
			ActionConfirm: fmt.Sprintf("%s/%s/%d", svr.commitUrl, gtid, i+1),
			ActionCancel:  fmt.Sprintf("%s/%s/%d", svr.rollbackUrl, gtid, i+1),
			Timeout:       time.Millisecond * 200,
			Retry:         time.Millisecond * 200,
		})
	}

	rm.requests[gtid] = req
	return req, nil
}
