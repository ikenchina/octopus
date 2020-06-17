package test

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/ikenchina/octopus/app/model"
	"github.com/ikenchina/octopus/common/errorutil"
	shttp "github.com/ikenchina/octopus/common/http"
	"github.com/ikenchina/octopus/config"
	tc "github.com/ikenchina/octopus/service"
	"github.com/ikenchina/octopus/service/saga"
)

func TestSagaSuite(t *testing.T) {
	suite.Run(t, new(_sagaSuite))
}

func (ss *_sagaSuite) TestCommit() {
	ss.runTc()
	defer ss.stopTc()
	sr, err := ss.rm.newSagaRequest()
	ss.Nil(err)
	_, err = ss.commit(sr)
	ss.Nil(err)
	na := <-ss.rm.notifyChan
	ss.checkServersData(sr.Gtid, true, []bool{true, true, true})
	ss.Equal(model.TxnStateCommitted, na.State)
}

func (ss *_sagaSuite) TestRollback() {
	ss.runTc()
	defer ss.stopTc()
	sr, err := ss.rm.newSagaRequest()
	ss.Nil(err)
	ss.rm.servers[1].timeout = 400 * time.Millisecond
	_, err = ss.commit(sr)
	ss.Nil(err)
	na := <-ss.rm.notifyChan

	ss.checkServersData(sr.Gtid, false, []bool{true, true, true})
	ss.checkServersData(sr.Gtid, true, []bool{true, true, false})

	ss.Equal(model.TxnStateAborted, na.State)
}

func (ss *_sagaSuite) TestRetryOk() {
	ss.runTc()
	defer ss.stopTc()
	sr, err := ss.rm.newSagaRequest()
	ss.Nil(err)

	ss.rm.servers[1].timeout = 300 * time.Millisecond
	go func() {
		_, err := ss.commit(sr)
		ss.Nil(err)
	}()
	<-ss.rm.servers[1].commitChan
	ss.rm.servers[1].timeout = 10 * time.Millisecond

	na := <-ss.rm.notifyChan
	ss.checkServersData(sr.Gtid, true, []bool{true, true, true})
	ss.checkServersData(sr.Gtid, false, []bool{false, false, false})
	ss.Equal(model.TxnStateCommitted, na.State)
}

func (ss *_sagaSuite) TestCron() {
	ss.runTc()

	sr, err := ss.rm.newSagaRequest()
	ss.Nil(err)
	sr.Branches[1].Commit.Retry.MaxRetry = 1000
	ss.rm.servers[1].timeout = 300 * time.Millisecond
	go func() {
		code, err := ss.commit(sr)
		ss.Equal(http.StatusInternalServerError, code)
		ss.NotNil(err)
	}()
	<-ss.rm.servers[1].commitChan
	ss.stopTc()
	ss.runTc()

	na := <-ss.rm.notifyChan
	ss.checkServersData(sr.Gtid, true, []bool{true, true, true})
	ss.checkServersData(sr.Gtid, false, []bool{false, false, false})
	ss.Equal(model.TxnStateCommitted, na.State)
	ss.stopTc()
}

type _sagaSuite struct {
	suite.Suite
	rm *sagaRmMock
	tc *tc.TcService
}

func (ss *_sagaSuite) checkServersData(gtid string, commit bool, expected []bool) {
	for i, svr := range ss.rm.servers {
		d, ok := svr.data[gtid]
		ss.True(ok)
		if commit {
			ss.Equal(expected[i], d.commit)
		} else {
			ss.Equal(expected[i], d.rollback)
		}
	}
}

func (ss *_sagaSuite) commit(sr *saga.SagaRequest) (int, error) {
	body, _ := json.Marshal(sr)
	code, err := shttp.Post(context.Background(), sr.Gtid, ss.rm.tcDomain+"/dtx/saga", string(body))
	if err != nil {
		return 0, err
	}
	if code < 200 || code >= 300 {
		return code, fmt.Errorf("status code : %d", code)
	}

	return code, nil
}

func (ss *_sagaSuite) get(id string) (*saga.SagaResponse, error) {
	sr := &saga.SagaResponse{}
	code, err := shttp.Get(context.Background(), id, fmt.Sprintf("%s/%s", ss.rm.tcDomain, id), sr)
	if err != nil {
		return nil, err
	}
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("status code : %d", code)
	}
	return sr, nil
}

func (ss *_sagaSuite) runTc() {
	if !flag.Parsed() {
		flag.Parse()
	}
	errorutil.PanicIfError(config.InitConfig(*configFile))
	ss.tc = tc.NewTc()
	ss.rm = newSagaRm(3, ":8083")
	go ss.rm.start()
	go ss.tc.Start()
	time.Sleep(time.Second)
}

func (ss *_sagaSuite) stopTc() {
	ss.tc.Stop()
	ss.rm.stop()
}
