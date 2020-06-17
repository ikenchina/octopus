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
	"github.com/ikenchina/octopus/service/tcc"
)

func TestTccSuite(t *testing.T) {
	suite.Run(t, new(_tccSuite))
}

func (ss *_tccSuite) TestCommit() {
	ss.runTc()
	defer ss.stopTc()
	sr, err := ss.rm.newTccRequest()
	ss.Nil(err)

	branches := sr.Branches
	sr.Branches = nil

	_, err = ss.prepare(sr)
	ss.Nil(err)

	for _, b := range branches {
		sr.Branches = []tcc.TccBranch{b}
		_, err = ss.register(sr)
		ss.Nil(err)
	}
	sr.Branches = nil

	_, err = ss.commit(sr)
	ss.Nil(err)
	na, err := ss.get(sr.Gtid)
	ss.Nil(err)
	ss.checkServersData(sr.Gtid, true, []bool{true, true, true})
	ss.Equal(model.TxnStateCommitted, na.State)
}

func (ss *_tccSuite) TestRollback() {
	ss.runTc()
	defer ss.stopTc()
	sr, err := ss.rm.newTccRequest()
	ss.Nil(err)

	_, err = ss.prepare(sr)
	ss.Nil(err)
	_, err = ss.register(sr)
	ss.Nil(err)

	_, err = ss.rollback(sr)
	ss.Nil(err)
	na, err := ss.get(sr.Gtid)
	ss.Nil(err)
	ss.checkServersData(sr.Gtid, false, []bool{true, true, true})
	ss.checkServersData(sr.Gtid, true, []bool{false, false, false})

	ss.Equal(model.TxnStateAborted, na.State)
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
		_, err = ss.rollback(sr)
		ss.Nil(err)
	}()

	code, err := ss.rollback(sr)
	ss.Nil(err)
	ss.Equal(http.StatusOK, code)

	na, err := ss.get(sr.Gtid)
	ss.Nil(err)
	ss.checkServersData(sr.Gtid, false, []bool{true, true, true})
	ss.checkServersData(sr.Gtid, true, []bool{false, false, false})

	ss.Equal(model.TxnStateAborted, na.State)
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
		sr.Branches = []tcc.TccBranch{b}
		_, err = ss.register(sr)
		ss.Nil(err)
	}
	sr.Branches = nil

	ss.rm.servers[1].timeout = 300 * time.Millisecond
	go func() {
		_, err = ss.commit(sr)
		ss.Nil(err)
	}()
	time.Sleep(800 * time.Millisecond)

	ss.rm.servers[1].timeout = 1 * time.Millisecond
	time.Sleep(200 * time.Millisecond)

	na, err := ss.get(sr.Gtid)
	ss.Nil(err)
	ss.checkServersData(sr.Gtid, true, []bool{true, true, true})
	ss.checkServersData(sr.Gtid, false, []bool{false, false, false})
	ss.Equal(model.TxnStateCommitted, na.State)
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
		sr.Branches = []tcc.TccBranch{b}
		_, err = ss.register(sr)
		ss.Nil(err)
	}
	sr.Branches = nil

	ss.rm.servers[1].timeout = 800 * time.Millisecond
	go func() {
		code, _ := ss.commit(sr)
		ss.Equal(http.StatusInternalServerError, code)
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
	ss.Equal(model.TxnStateCommitted, na.State)
}

type _tccSuite struct {
	suite.Suite
	rm *tccRmMock
	tc *tc.TcService
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

func (ss *_tccSuite) prepare(sr *tcc.TccRequest) (int, error) {
	body, _ := json.Marshal(sr)
	code, err := shttp.Post(context.Background(), sr.Gtid, ss.rm.tcDomain+"/dtx/tcc", string(body))
	if err != nil {
		return 0, err
	}
	if code < 200 || code >= 300 {
		return code, fmt.Errorf("status code : %d", code)
	}

	return code, nil
}

func (ss *_tccSuite) register(sr *tcc.TccRequest) (int, error) {
	body, _ := json.Marshal(sr)
	url := fmt.Sprintf(ss.rm.tcDomain+"/dtx/tcc/%s", sr.Gtid)
	code, err := shttp.Post(context.Background(), sr.Gtid, url, string(body))
	if err != nil {
		return 0, err
	}
	if code < 200 || code >= 300 {
		return code, fmt.Errorf("status code : %d", code)
	}

	return code, nil
}

func (ss *_tccSuite) commit(sr *tcc.TccRequest) (int, error) {
	body, _ := json.Marshal(sr)
	url := fmt.Sprintf(ss.rm.tcDomain+"/dtx/tcc/%s", sr.Gtid)
	code, err := shttp.Put(context.Background(), sr.Gtid, url, string(body))
	if err != nil {
		return 0, err
	}
	if code < 200 || code >= 300 {
		return code, fmt.Errorf("status code : %d", code)
	}

	return code, nil
}

func (ss *_tccSuite) rollback(sr *tcc.TccRequest) (int, error) {
	url := fmt.Sprintf(ss.rm.tcDomain+"/dtx/tcc/%s", sr.Gtid)
	code, err := shttp.Delete(context.Background(), sr.Gtid, url)
	if err != nil {
		return 0, err
	}
	if code < 200 || code >= 300 {
		return code, fmt.Errorf("status code : %d", code)
	}

	return code, nil
}

func (ss *_tccSuite) get(id string) (*tcc.TccResponse, error) {
	sr := &tcc.TccResponse{}
	url := fmt.Sprintf(ss.rm.tcDomain+"/dtx/tcc/%s", id)
	code, err := shttp.Get(context.Background(), id, url, sr)
	if err != nil {
		return nil, err
	}
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("status code : %d", code)
	}
	return sr, nil
}

func (ss *_tccSuite) runTc() {
	if !flag.Parsed() {
		flag.Parse()
	}
	errorutil.PanicIfError(config.InitConfig(*configFile))
	ss.tc = tc.NewTc()
	ss.rm = newTccRm(3, ":8083")
	go ss.rm.start()
	go ss.tc.Start()
	time.Sleep(time.Second)
}

func (ss *_tccSuite) stopTc() {
	ss.tc.Stop()
	ss.rm.stop()
}
