package model

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/ikenchina/octopus/common/slice"
	"github.com/ikenchina/octopus/define"
	"github.com/stretchr/testify/suite"
)

func TestSuite(t *testing.T) {
	suite.Run(t, new(_Suite))
}

type _Suite struct {
	suite.Suite
	dsn     string
	driver  string
	timeout time.Duration
	lessee  string
	random  rand.Rand
}

func (s *_Suite) TestSave() {
	ms, err := NewModelStorage(s.driver, s.dsn, s.timeout, 1, 1, s.lessee)
	s.Nil(err)

	saga := s.newSagaTxn(2)

	err = ms.Save(context.TODO(), saga)
	s.Nil(err)

	err = ms.Save(context.TODO(), saga)
	s.NotNil(err)

	getSaga, err := ms.GetByGtid(context.Background(), saga.Gtid)
	s.Nil(err)
	s.True(compareSaga(saga, getSaga))
}

func (s *_Suite) TestExist() {
	ms, err := NewModelStorage(s.driver, s.dsn, s.timeout, 1, 1, s.lessee)
	s.Nil(err)

	saga := s.newSagaTxn(2)

	err = ms.Save(context.TODO(), saga)
	s.Nil(err)

	err = ms.Exist(context.TODO(), saga.Gtid)
	s.Nil(err)

	err = ms.Exist(context.TODO(), saga.Gtid+"_test")
	s.Equal(ErrNotExist, err)
}

func (s *_Suite) TestUpdate() {
	ms, err := NewModelStorage(s.driver, s.dsn, s.timeout, 1, 1, s.lessee)
	s.Nil(err)

	// invalid lease
	{
		txn := s.newSagaTxn(2)
		txn.Lessee = "test_invalid_leessee"
		s.Nil(ms.Save(context.TODO(), txn))
		txn.Branches[0].SetState(define.TxnStateCommitted)
		err = ms.Update(context.Background(), txn)
		s.Equal(ErrInvalidLessee, err)
	}

	txn := s.newSagaTxn(2)
	s.Nil(ms.Save(context.TODO(), txn))

	for _, sub := range txn.Branches {
		sub.SetState(define.TxnStateCommitted)
	}

	txn.SetState(define.TxnStateCommitted)

	err = ms.Update(context.TODO(), txn)
	s.Nil(err)

	getSaga, err := ms.GetByGtid(context.Background(), txn.Gtid)
	s.Nil(err)

	s.True(compareSaga(txn, getSaga))
}

func (s *_Suite) TestUpdateInvalidLessee() {
	ms, err := NewModelStorage(s.driver, s.dsn, s.timeout, 1, 1, s.lessee)
	s.Nil(err)

	saga := s.newSagaTxn(2)
	saga.Lessee = "test_invalid_lessee"

	err = ms.Save(context.TODO(), saga)
	s.Nil(err)

	saga.SetLessee(s.lessee)

	err = ms.Update(context.Background(), saga)
	s.Equal(ErrInvalidLessee, err)
}

func (s *_Suite) TestUpdateBranch() {
	ms, err := NewModelStorage(s.driver, s.dsn, s.timeout, 1, 1, s.lessee)
	s.Nil(err)

	// invalid lease
	{
		txn := s.newSagaTxn(2)
		txn.Lessee = "test_invalid_leessee"
		s.Nil(ms.Save(context.TODO(), txn))
		txn.Branches[0].SetState(define.TxnStateCommitted)
		err = ms.UpdateBranch(context.Background(), txn.Branches[0])
		s.Equal(ErrInvalidLessee, err)
	}

	//
	txn := s.newSagaTxn(2)
	s.Nil(ms.Save(context.TODO(), txn))
	txn.Branches[0].SetState(define.TxnStateCommitted)
	err = ms.UpdateBranch(context.Background(), txn.Branches[0])
	s.Nil(err)

	gtxn, err := ms.GetByGtid(context.Background(), txn.Gtid)
	s.Nil(err)
	s.Equal(define.TxnStateCommitted, gtxn.Branches[0].State)
}

func (s *_Suite) TestQueryNotExist() {
	ms, err := NewModelStorage(s.driver, s.dsn, s.timeout, 1, 1, s.lessee)
	s.Nil(err)

	saga, err := ms.GetByGtid(context.Background(), "test_gtid_invalid")
	s.Equal(ErrNotExist, err)
	s.Nil(saga)
}

func (s *_Suite) TestFindPreparedExpired() {
	ms, err := NewModelStorage(s.driver, s.dsn, s.timeout, 1, 1, s.lessee)
	s.Nil(err)
	var ts Txns
	for i := 0; i < 10; i++ {
		ts = append(ts, s.newSagaTxn(2))
	}

	// drain
	for {
		ets, err := ms.FindPreparedExpired(context.Background(), ts[0].TxnType, 100)
		s.Nil(err)
		if len(ets) == 0 {
			break
		}
		for _, t := range ets {
			t.SetState(define.TxnStateAborted)
			s.Nil(ms.Update(context.Background(), t))
		}
	}

	now := time.Now()
	expiredTime := now.Add(-10 * time.Second)
	for i, txn := range ts {
		if i < 3 {
			txn.ExpireTime = now.Add(10 * time.Second)
		} else {
			txn.ExpireTime = expiredTime
		}
		txn.SetState(define.TxnStatePrepared)
		s.Nil(ms.Save(context.Background(), txn))
	}

	limit := 4
	for i := 0; i < 2; i++ {
		ets, err := ms.FindPreparedExpired(context.Background(), ts[0].TxnType, limit)
		s.Nil(err)
		idc := int64(math.MaxInt64)
		for _, t := range ets {
			s.True(compareTxnTime(t.ExpireTime, expiredTime))
			s.True(t.Id < idc)
			s.Equal(define.TxnStatePrepared, t.State)
			idc = t.Id
			t.SetState(define.TxnStateAborted)
			s.Nil(ms.Update(context.Background(), t))
		}
		if i == 1 {
			s.Equal(3, len(ets))
		}
	}
}

func (s *_Suite) TestFindRunningLeaseExpired() {
	states := []string{define.TxnStateRolling, define.TxnStateCommitting}

	ms, err := NewModelStorage(s.driver, s.dsn, s.timeout, 1, 1, s.lessee)
	s.Nil(err)
	var ts Txns
	for i := 0; i < 10; i++ {
		ts = append(ts, s.newSagaTxn(2))
	}

	for {
		ets, err := ms.FindRunningLeaseExpired(context.Background(), ts[0].TxnType, 100)
		s.Nil(err)
		if len(ets) == 0 {
			break
		}
		for _, t := range ets {
			t.SetState(define.TxnStateAborted)
			s.Nil(ms.Update(context.Background(), t))
		}
	}

	now := time.Now()
	expiredTime := now.Add(-10 * time.Second)
	for i, txn := range ts {
		if i < 3 {
			txn.LeaseExpireTime = now.Add(10 * time.Second)
		} else {
			txn.LeaseExpireTime = expiredTime
		}
		txn.SetState(states[i%len(states)])
		s.Nil(ms.Save(context.Background(), txn))
	}

	limit := 4
	for i := 0; i < 2; i++ {
		ets, err := ms.FindRunningLeaseExpired(context.Background(), ts[0].TxnType, limit)
		s.Nil(err)
		idc := int64(math.MaxInt64)
		for _, t := range ets {
			s.True(compareTxnTime(t.LeaseExpireTime, expiredTime))
			s.True(t.LeaseExpireTime.Unix() <= idc)
			s.True(slice.Contain(states, t.State))
			idc = t.LeaseExpireTime.Unix()
			t.SetState(define.TxnStateAborted)
			s.Nil(ms.Update(context.Background(), t))
		}
		if i == 1 {
			s.Equal(3, len(ets))
		}
	}
}

func (s *_Suite) TestGrantLease() {
	states := []string{define.TxnStatePrepared, define.TxnStateRolling, define.TxnStateCommitting}

	ms, err := NewModelStorage(s.driver, s.dsn, s.timeout, 1, 1, s.lessee)
	s.Nil(err)
	var ts Txns
	for i := 0; i < 10; i++ {
		ts = append(ts, s.newSagaTxn(2))
	}

	now := time.Now()
	expiredTime := now.Add(-10 * time.Second)
	for i, txn := range ts {
		if i%2 == 0 {
			txn.LeaseExpireTime = now.Add(10 * time.Second)
		} else {
			txn.LeaseExpireTime = expiredTime
		}
		txn.Lessee = "Test_other_lessee"
		txn.SetState(states[i%len(states)])
		s.Nil(ms.Save(context.Background(), txn))
	}

	for i, txn := range ts {
		if i%2 == 0 {
			s.Equal(ErrInvalidLessee, ms.GrantLease(context.Background(), txn, txn.LeaseExpireTime.Sub(time.Now())))
		} else {
			s.Nil(ms.GrantLease(context.Background(), txn, txn.LeaseExpireTime.Sub(time.Now())))
		}
	}
}

func (s *_Suite) TestUpdateConditions() {

	ms, err := NewModelStorage(s.driver, s.dsn, s.timeout, 1, 1, s.lessee)
	s.Nil(err)

	// invalid lease
	{
		txn := s.newSagaTxn(2)
		txn.Lessee = "test_invalid_lessee"
		s.Nil(ms.Save(context.TODO(), txn))
		txn.Branches[0].SetState(define.TxnStateCommitted)
		err = ms.UpdateConditions(context.Background(), txn, nil)
		s.Equal(ErrInvalidLessee, err)
	}

	txn := s.newSagaTxn(2)
	s.Nil(ms.Save(context.TODO(), txn))

	txn.SetState(define.TxnStateCommitting)
	s.NotNil(ms.UpdateConditions(context.Background(), txn,
		func(oldTxn *Txn) error {
			return fmt.Errorf("test")
		}))
	gtxn, err := ms.GetByGtid(context.Background(), txn.Gtid)
	s.Nil(err)
	s.NotEqual(define.TxnStateCommitting, gtxn.State)

	s.Nil(ms.UpdateConditions(context.Background(), txn,
		func(oldTxn *Txn) error {
			return nil
		}))
	gtxn, err = ms.GetByGtid(context.Background(), txn.Gtid)
	s.Nil(err)
	s.Equal(define.TxnStateCommitting, gtxn.State)
}

func (s *_Suite) TestRegisterBranches() {

	ms, err := NewModelStorage(s.driver, s.dsn, s.timeout, 1, 1, s.lessee)
	s.Nil(err)

	// register expired record
	{
		txn := s.newSagaTxn(2)
		txn.ExpireTime = time.Now().Add(-10 * time.Second)
		bb := txn.Branches
		txn.Branches = nil
		s.Nil(ms.Save(context.TODO(), txn))
		s.Equal(ErrInvalidLessee, ms.RegisterBranches(context.Background(), bb))
	}

	txn := s.newSagaTxn(2)
	branches := txn.Branches
	txn.Branches = nil
	s.Nil(ms.Save(context.TODO(), txn))
	s.Nil(ms.RegisterBranches(context.Background(), branches))
}

func (s *_Suite) TestGrantLeaseIncBranchCheckState() {
	ms, err := NewModelStorage(s.driver, s.dsn, s.timeout, 1, 1, s.lessee)
	s.Nil(err)
	txn := s.newSagaTxn(2)
	s.Nil(ms.Save(context.TODO(), txn))
	branch := txn.Branches[0]

	states := []string{define.TxnStatePrepared}
	s.Nil(ms.GrantLeaseIncBranchCheckState(context.Background(), txn, branch, 120*time.Second, states))
	txn2, err := ms.GetByGtid(context.Background(), txn.Gtid)
	s.Nil(err)
	s.Greater(txn2.LeaseExpireTime, time.Now().Add(118*time.Second))
	s.Equal(2, txn2.Branches[0].TryCount)
}

func (s *_Suite) TestCleanExpiredTxns() {
	ms, err := NewModelStorage(s.driver, s.dsn, s.timeout, 1, 1, s.lessee)
	s.Nil(err)
	states := []string{define.TxnStateAborted, define.TxnStateCommitted}

	for {
		now := time.Now()
		expiredTime := now.Add(10 * time.Second)
		expired, err := ms.CleanExpiredTxns(context.Background(), define.TxnTypeSaga, expiredTime, 20)
		s.Nil(err)
		if len(expired) == 0 {
			break
		}
	}

	for i := 0; i < 60; i++ {
		txn := s.newSagaTxn(2)
		if i < 10 {
			txn.TxnType = define.TxnTypeTcc
		} else if i >= 10 && i < 20 {
			txn.State = define.TxnStatePrepared
		} else if i >= 20 && i < 30 {
			txn.State = define.TxnStateAborted
		} else if i >= 30 && i < 50 {
			txn.State = define.TxnStateCommitted
		} else if i >= 50 {
			txn.ExpireTime = time.Now().Add(1 * time.Hour)
		}
		s.Nil(ms.Save(context.TODO(), txn))
	}
	now := time.Now()
	expiredTime := now.Add(10 * time.Second)
	fn := func(expectedLimit int) {
		expired, err := ms.CleanExpiredTxns(context.Background(), define.TxnTypeSaga, expiredTime, 20)
		s.Nil(err)
		s.Equal(expectedLimit, len(expired))
		for _, e := range expired {
			s.True(slice.Contain(states, e.State))
			s.Equal(define.TxnTypeSaga, e.TxnType)
			s.Less(e.ExpireTime, expiredTime)
		}
	}
	fn(20)
	fn(10)
}

///
///
///
///
///

func (s *_Suite) SetupSuite() {
	// docker run --name pg -e POSTGRES_PASSWORD=pg -d postgres
	dsn := flag.String("db_dsn", "postgresql://dtx_user:dtx_pass@127.0.0.1:5432/dtx?connect_timeout=3", "database dsn")
	driver := flag.String("driver", "postgresql", "database driver")
	timeout := flag.Duration("timeout", 1*time.Second, "timeout")
	lessee := flag.String("lessee", "test_user", "lessee")
	flag.Parse()
	s.dsn = *dsn
	s.driver = *driver
	s.timeout = *timeout
	s.lessee = *lessee

	s.random = *rand.New(rand.NewSource(time.Now().Unix()))
}

func compareTxnTime(a, b time.Time) bool {
	return a.Unix() == b.Unix()
}

func compareSaga(a, b *Txn) bool {

	// aa, _ := json.Marshal(a)
	// bb, _ := json.Marshal(b)
	// return bytes.Equal(aa, bb)

	if a.Id != b.Id || a.Gtid != b.Gtid || a.Business != b.Business ||
		a.State != b.State || a.Lessee != b.Lessee || a.TxnType != b.TxnType ||
		a.CallType != b.CallType || a.NotifyAction != b.NotifyAction || a.NotifyTimeout != b.NotifyTimeout ||
		a.NotifyRetry != b.NotifyRetry || a.NotifyCount != b.NotifyCount ||
		a.ParallelExecution != b.ParallelExecution {
		return false
	}

	if !compareTxnTime(a.UpdatedTime, b.UpdatedTime) || !compareTxnTime(a.CreatedTime, b.CreatedTime) ||
		!compareTxnTime(a.LeaseExpireTime, b.LeaseExpireTime) || !compareTxnTime(a.ExpireTime, b.ExpireTime) {
		return false
	}

	if len(a.Branches) != len(b.Branches) {
		return false
	}

	for i := 0; i < len(a.Branches); i++ {
		aa := a.Branches[i]
		bb := b.Branches[i]

		if aa.Id != bb.Id || aa.Gtid != bb.Gtid || aa.Bid != bb.Bid || aa.Action != bb.Action ||
			!bytes.Equal(aa.Payload, bb.Payload) || aa.Timeout != bb.Timeout ||
			aa.State != bb.State || aa.Retry.MaxRetry != bb.Retry.MaxRetry ||
			aa.TryCount != bb.TryCount {
			return false
		}

		if !compareTxnTime(aa.UpdatedTime, bb.UpdatedTime) || !compareTxnTime(aa.CreatedTime, bb.CreatedTime) {
			return false
		}
		if aa.Retry.Constant != nil && bb.Retry.Constant != nil {
			if aa.Retry.Constant.Duration != bb.Retry.Constant.Duration {
				return false
			}
		}

	}

	return true
}

func (s *_Suite) newSagaTxn(subCount int) *Txn {

	txn := &Txn{
		//Id:                int64(s.random.Int31()),
		Gtid:              fmt.Sprintf("test_gtid_%v", s.random.Int31()),
		Business:          "test_bz",
		State:             define.TxnStatePrepared,
		TxnType:           TxnTypeSaga,
		UpdatedTime:       time.Now(),
		CreatedTime:       time.Now(),
		LeaseExpireTime:   time.Now().Add(2 * time.Second),
		ExpireTime:        time.Now().Add(2 * time.Second),
		Lessee:            s.lessee,
		CallType:          TxnCallTypeSync,
		NotifyAction:      "http://test/notify",
		NotifyTimeout:     1 * time.Second,
		NotifyRetry:       2 * time.Second,
		ParallelExecution: false,
	}

	for i := 0; i < subCount; i++ {
		sub := &Branch{
			//Id:                  int64(i),
			Gtid:        txn.Gtid,
			Bid:         i,
			BranchType:  BranchTypeCommit,
			Action:      fmt.Sprintf("http://service_%v/saga", i),
			Payload:     []byte(fmt.Sprintf(`{"Gtid":%s, "Stid":%d}`, txn.Gtid, i)),
			Timeout:     1 * time.Second,
			State:       define.TxnStatePrepared,
			UpdatedTime: time.Now(),
			CreatedTime: time.Now(),
			TryCount:    1,
		}
		sub.Retry.Constant = &RetryConstant{
			Duration: 2 * time.Second,
		}
		sub.Retry.MaxRetry = 3

		com := &Branch{
			//Id:                  int64(i),
			Gtid:        txn.Gtid,
			Bid:         i,
			BranchType:  BranchTypeCompensation,
			Action:      fmt.Sprintf("http://service_%v/saga/rollback", i),
			Payload:     []byte(fmt.Sprintf(`{"Gtid":%s, "Stid":%d}`, txn.Gtid, i)),
			Timeout:     1 * time.Second,
			State:       define.TxnStatePrepared,
			UpdatedTime: time.Now(),
			CreatedTime: time.Now(),
			TryCount:    1,
		}
		sub.Retry.Constant = &RetryConstant{
			Duration: 2 * time.Second,
		}

		txn.Branches = append(txn.Branches, sub, com)
	}
	return txn
}
