package test

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	tcc_cli "github.com/ikenchina/octopus/ap/tcc"
	"github.com/ikenchina/octopus/common/errorutil"
	shttp "github.com/ikenchina/octopus/common/http"
	"github.com/ikenchina/octopus/define"
	"github.com/ikenchina/octopus/tc/config"
	tc "github.com/ikenchina/octopus/tc/service"
)

func TestTccSuite(t *testing.T) {
	suite.Run(t, new(_tccSuite))
}

func (ss *_tccSuite) TestCommit() {
	ss.runTc()
	defer ss.stopTc()

	sr, err := ss.rm.newTccRequest()
	ss.Nil(err)
	_, err = tcc_cli.TccTransaction(context.Background(), ss.rm.tcDomain, time.Now().Add(time.Second),
		func(t *tcc_cli.Transaction, gtid string) error {
			sr.Gtid = gtid
			for i, branch := range sr.Branches {
				tryUrl := fmt.Sprintf("%s%s/%s/%d", ss.rm.rmDomain, ss.rm.servers[i].basePath, gtid, branch.BranchId)
				branch.ActionConfirm = tryUrl
				branch.ActionCancel = tryUrl
				_, err = t.Try(branch.BranchId, tryUrl, branch.ActionConfirm, branch.ActionCancel, branch.Payload)
				if err != nil {
					return err
				}
			}
			return nil
		})
	ss.Nil(err)

	na, err := ss.get(sr.Gtid)
	ss.Nil(err)
	ss.checkServersData(sr.Gtid, true, []bool{true, true, true})
	ss.Equal(define.TxnStateCommitted, na.State)
}

func (ss *_tccSuite) TestRollback() {
	ss.runTc()
	defer ss.stopTc()
	sr, err := ss.rm.newTccRequest()
	ss.Nil(err)

	_, err = tcc_cli.TccTransaction(context.Background(), ss.rm.tcDomain, time.Now().Add(time.Second),
		func(t *tcc_cli.Transaction, gtid string) error {
			sr.Gtid = gtid
			for i, branch := range sr.Branches {
				tryUrl := fmt.Sprintf("%s%s/%s/%d", ss.rm.rmDomain, ss.rm.servers[i].basePath, gtid, branch.BranchId)
				branch.ActionConfirm = tryUrl
				branch.ActionCancel = tryUrl
				if i == len(sr.Branches)-1 {
					ss.rm.servers[i].actionErr = true
				}
				_, err = t.Try(branch.BranchId, tryUrl, branch.ActionConfirm, branch.ActionCancel, branch.Payload)
				if err != nil {
					return err
				}
			}
			return nil
		})
	ss.Nil(err)

	na, err := ss.get(sr.Gtid)
	ss.Nil(err)
	ss.checkServersData(sr.Gtid, false, []bool{true, true, true})
	ss.checkServersData(sr.Gtid, true, []bool{false, false, false})

	ss.Equal(define.TxnStateAborted, na.State)
}

func (ss *_tccSuite) TestReRollback() {
	ss.runTc()
	defer ss.stopTc()
	sr, err := ss.rm.newTccRequest()
	ss.Nil(err)

	_, err = ss.prepare(sr)
	ss.Nil(err)
	_, err = ss.register(sr)
	ss.Nil(err)

	ss.rm.servers[0].rollbackTimeout = 400 * time.Millisecond
	go func() {
		time.Sleep(200 * time.Millisecond)
		ss.rm.servers[0].rollbackTimeout = 0 * time.Millisecond
		_, err = ss.rollback(sr.Gtid)
		ss.Nil(err)
	}()

	_, err = ss.rollback(sr.Gtid)
	ss.Nil(err)

	na, err := ss.get(sr.Gtid)
	ss.Nil(err)
	ss.checkServersData(sr.Gtid, false, []bool{true, true, true})
	ss.checkServersData(sr.Gtid, true, []bool{false, false, false})

	ss.Equal(define.TxnStateAborted, na.State)
}

func (ss *_tccSuite) TestRetryOk() {
	ss.runTc()
	defer ss.stopTc()
	sr, err := ss.rm.newTccRequest()
	ss.Nil(err)

	branches := sr.Branches
	sr.Branches = nil

	_, err = ss.prepare(sr)
	ss.Nil(err)

	for _, b := range branches {
		sr.Branches = []define.TccBranch{b}
		_, err = ss.register(sr)
		ss.Nil(err)
	}
	sr.Branches = nil

	ss.rm.servers[1].timeout = 300 * time.Millisecond
	go func() {
		_, err = ss.commit(sr.Gtid)
		ss.Nil(err)
	}()
	time.Sleep(800 * time.Millisecond)

	ss.rm.servers[1].timeout = 1 * time.Millisecond
	time.Sleep(200 * time.Millisecond)

	na, err := ss.get(sr.Gtid)
	ss.Nil(err)
	ss.checkServersData(sr.Gtid, true, []bool{true, true, true})
	ss.checkServersData(sr.Gtid, false, []bool{false, false, false})
	ss.Equal(define.TxnStateCommitted, na.State)
}

func (ss *_tccSuite) TestCron() {
	ss.runTc()
	sr, err := ss.rm.newTccRequest()
	ss.Nil(err)

	branches := sr.Branches
	sr.Branches = nil
	_, err = ss.prepare(sr)
	ss.Nil(err)
	for _, b := range branches {
		sr.Branches = []define.TccBranch{b}
		_, err = ss.register(sr)
		ss.Nil(err)
	}
	sr.Branches = nil

	ss.rm.servers[1].timeout = 800 * time.Millisecond
	go func() {
		_, err := ss.commit(sr.Gtid)
		ss.NotNil(err)
	}()
	time.Sleep(300 * time.Millisecond)
	ss.stopTc()

	ss.rm.servers[1].timeout = 1 * time.Millisecond
	ss.runTc()
	time.Sleep(200 * time.Millisecond)

	na, err := ss.get(sr.Gtid)
	ss.Nil(err)
	ss.checkServersData(sr.Gtid, true, []bool{true, true, true})
	ss.checkServersData(sr.Gtid, false, []bool{false, false, false})
	ss.Equal(define.TxnStateCommitted, na.State)
}

type _tccSuite struct {
	suite.Suite
	rm  *tccRmMock
	tc  *tc.TcService
	cli tcc_cli.Client
}

func (ss *_tccSuite) checkServersData(gtid string, commit bool, expected []bool) {
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

func (ss *_tccSuite) prepare(sr *define.TccRequest) (int, error) {
	body, _ := json.Marshal(sr)
	code, err := shttp.PostJson(context.Background(), sr.Gtid, ss.rm.tcDomain+"/dtx/tcc", string(body), nil)
	if err != nil {
		return 0, err
	}
	if code < 200 || code >= 300 {
		return code, fmt.Errorf("status code : %d", code)
	}

	return code, nil
}

func (ss *_tccSuite) register(sr *define.TccRequest) (int, error) {
	body, _ := json.Marshal(sr)
	url := fmt.Sprintf(ss.rm.tcDomain+"/dtx/tcc/%s", sr.Gtid)
	code, err := shttp.PostJson(context.Background(), sr.Gtid, url, string(body), nil)
	if err != nil {
		return 0, err
	}
	if code < 200 || code >= 300 {
		return code, fmt.Errorf("status code : %d", code)
	}

	return code, nil
}

func (ss *_tccSuite) commit(gtid string) (*define.TccResponse, error) {
	return ss.cli.Confirm(context.Background(), gtid)
}

func (ss *_tccSuite) rollback(gtid string) (*define.TccResponse, error) {
	return ss.cli.Cancel(context.Background(), gtid)
}

func (ss *_tccSuite) get(id string) (*define.TccResponse, error) {
	return ss.cli.Get(context.Background(), id)
}

func (ss *_tccSuite) runTc() {
	if !flag.Parsed() {
		flag.Parse()
	}
	errorutil.PanicIfError(config.InitConfig(*configFile))
	ss.tc = tc.NewTc()
	ss.rm = newTccRm(3, ":8083")
	ss.cli.TcServer = ss.rm.tcDomain
	go ss.rm.start()
	go ss.tc.Start()
	time.Sleep(time.Second)
}

func (ss *_tccSuite) stopTc() {
	ss.tc.Stop()
	ss.rm.stop()
}
