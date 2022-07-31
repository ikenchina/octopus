package test

import (
	"context"
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	saga_client "github.com/ikenchina/octopus/ap/saga"
	"github.com/ikenchina/octopus/common/errorutil"
	shttp "github.com/ikenchina/octopus/common/http"
	"github.com/ikenchina/octopus/define"
	"github.com/ikenchina/octopus/tc/config"
	tc "github.com/ikenchina/octopus/tc/service"
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
	ss.Equal(define.TxnStateCommitted, na.State)
}

func (ss *_sagaSuite) TestCommitWrapper() {
	ss.runTc()
	defer ss.stopTc()
	sr, err := ss.rm.newSagaRequest()
	ss.Nil(err)

	_, err = saga_client.SagaTransaction(context.Background(), ss.rm.tcDomain, time.Now().Add(1*time.Minute), func(t *saga_client.Transaction, gtid string) error {
		for i, bb := range sr.Branches {
			t.AddHttpBranch(i+1, bb.Commit.Action, bb.Compensation.Action, bb.Payload)
		}
		t.SetHttpNotify(sr.Notify.Action, sr.Notify.Timeout, sr.Notify.Retry)
		return nil
	})
	ss.Nil(err)

	na := <-ss.rm.notifyChan
	ss.checkServersData(sr.Gtid, true, []bool{true, true, true})
	ss.Equal(define.TxnStateCommitted, na.State)
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

	ss.Equal(define.TxnStateAborted, na.State)
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
	ss.Equal(define.TxnStateCommitted, na.State)
}

func (ss *_sagaSuite) TestCron() {
	ss.runTc()

	sr, err := ss.rm.newSagaRequest()
	ss.Nil(err)
	sr.Branches[1].Commit.Retry.MaxRetry = 1000
	ss.rm.servers[1].timeout = 300 * time.Millisecond
	go func() {
		resp, err := ss.commit(sr)
		ss.Nil(err)
		ss.Equal(define.TxnStatePrepared, resp.State)
	}()
	<-ss.rm.servers[1].commitChan
	ss.stopTc()
	ss.runTc()

	na := <-ss.rm.notifyChan
	ss.checkServersData(sr.Gtid, true, []bool{true, true, true})
	ss.checkServersData(sr.Gtid, false, []bool{false, false, false})
	ss.Equal(define.TxnStateCommitted, na.State)
	ss.stopTc()
}

type _sagaSuite struct {
	suite.Suite
	rm  *sagaRmMock
	tc  *tc.TcService
	cli saga_client.Client
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

func (ss *_sagaSuite) commit(sr *define.SagaRequest) (*define.SagaResponse, error) {
	return ss.cli.Commit(context.Background(), sr)
}

func (ss *_sagaSuite) get(id string) (*define.SagaResponse, error) {
	sr := &define.SagaResponse{}
	code, err := shttp.GetJson(context.Background(), id, fmt.Sprintf("%s/%s", ss.rm.tcDomain, id), sr)
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

	ss.cli.TcServer = ss.rm.tcDomain

	go ss.rm.start()
	go ss.tc.Start()
	time.Sleep(time.Second)
}

func (ss *_sagaSuite) stopTc() {
	ss.tc.Stop()
	ss.rm.stop()
}
