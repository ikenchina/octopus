package model

import (
	"context"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/ikenchina/octopus/common/slice"
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
	ms, err := NewModelStorage(s.driver, s.dsn, s.timeout, s.lessee)
	s.Nil(err)

	saga := s.newTxn(2)

	err = ms.Save(context.TODO(), saga)
	s.Nil(err)

	err = ms.Save(context.TODO(), saga)
	s.NotNil(err)

	getSaga, err := ms.GetByGtid(context.Background(), saga.Gtid)
	s.Nil(err)

	s.True(compareSaga(saga, getSaga))
}

func (s *_Suite) TestExist() {
	ms, err := NewModelStorage(s.driver, s.dsn, s.timeout, s.lessee)
	s.Nil(err)

	saga := s.newTxn(2)

	err = ms.Save(context.TODO(), saga)
	s.Nil(err)

	err = ms.Exist(context.TODO(), saga.Gtid)
	s.Nil(err)

	err = ms.Exist(context.TODO(), saga.Gtid+"_test")
	s.Equal(ErrNotExist, err)
}

func (s *_Suite) TestUpdate() {
	ms, err := NewModelStorage(s.driver, s.dsn, s.timeout, s.lessee)
	s.Nil(err)

	// invalid lease
	{
		txn := s.newTxn(2)
		txn.Lessee = "test_invalid_leessee"
		s.Nil(ms.Save(context.TODO(), txn))
		txn.Branches[0].SetState(TxnStateCommitted)
		err = ms.Update(context.Background(), txn)
		s.Equal(ErrInvalidLessee, err)
	}

	txn := s.newTxn(2)
	s.Nil(ms.Save(context.TODO(), txn))

	for _, sub := range txn.Branches {
		sub.SetState(TxnStateCommitted)
	}

	txn.SetState(TxnStateCommitted)

	err = ms.Update(context.TODO(), txn)
	s.Nil(err)

	getSaga, err := ms.GetByGtid(context.Background(), txn.Gtid)
	s.Nil(err)

	s.True(compareSaga(txn, getSaga))
}

func (s *_Suite) TestUpdateInvalidLessee() {
	ms, err := NewModelStorage(s.driver, s.dsn, s.timeout, s.lessee)
	s.Nil(err)

	saga := s.newTxn(2)
	saga.Lessee = "test_invalid_lessee"

	err = ms.Save(context.TODO(), saga)
	s.Nil(err)

	saga.SetLessee(s.lessee)

	err = ms.Update(context.Background(), saga)
	s.Equal(ErrInvalidLessee, err)
}

func (s *_Suite) TestUpdateBranch() {
	ms, err := NewModelStorage(s.driver, s.dsn, s.timeout, s.lessee)
	s.Nil(err)

	// invalid lease
	{
		txn := s.newTxn(2)
		txn.Lessee = "test_invalid_leessee"
		s.Nil(ms.Save(context.TODO(), txn))
		txn.Branches[0].SetState(TxnStateCommitted)
		err = ms.UpdateBranch(context.Background(), txn.Branches[0])
		s.Equal(ErrInvalidLessee, err)
	}

	//
	txn := s.newTxn(2)
	s.Nil(ms.Save(context.TODO(), txn))
	txn.Branches[0].SetState(TxnStateCommitted)
	err = ms.UpdateBranch(context.Background(), txn.Branches[0])
	s.Nil(err)

	gtxn, err := ms.GetByGtid(context.Background(), txn.Gtid)
	s.Nil(err)
	s.Equal(TxnStateCommitted, gtxn.Branches[0].State)
}

func (s *_Suite) TestQueryNotExist() {
	ms, err := NewModelStorage(s.driver, s.dsn, s.timeout, s.lessee)
	s.Nil(err)

	saga, err := ms.GetByGtid(context.Background(), "test_gtid_invalid")
	s.Equal(ErrNotExist, err)
	s.Nil(saga)
}

func (s *_Suite) TestFindTxnExpired() {
	states := []string{TxnStatePrepared, TxnStateFailed, TxnStateCommitting}

	ms, err := NewModelStorage(s.driver, s.dsn, s.timeout, s.lessee)
	s.Nil(err)
	var ts Txns
	for i := 0; i < 10; i++ {
		ts = append(ts, s.newTxn(2))
	}

	for {
		ets, err := ms.FindTxnExpired(context.Background(), ts[0].TxnType, 100)
		s.Nil(err)
		if len(ets) == 0 {
			break
		}
		for _, t := range ets {
			t.SetState(TxnStateAborted)
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
		txn.SetState(states[i%len(states)])
		s.Nil(ms.Save(context.Background(), txn))
	}

	limit := 4
	for i := 0; i < 2; i++ {
		ets, err := ms.FindTxnExpired(context.Background(), ts[0].TxnType, limit)
		s.Nil(err)
		idc := int64(math.MaxInt64)
		for _, t := range ets {
			s.True(compareTxnTime(t.ExpireTime, expiredTime))
			s.True(t.Id < idc)
			s.True(slice.Contain(states, t.State))
			idc = t.Id
			t.SetState(TxnStateAborted)
			s.Nil(ms.Update(context.Background(), t))
		}
		if i == 1 {
			s.Equal(3, len(ets))
		}
	}
}

func (s *_Suite) TestFindLeaseExpired() {
	states := []string{TxnStatePrepared, TxnStateFailed, TxnStateCommitting}

	ms, err := NewModelStorage(s.driver, s.dsn, s.timeout, s.lessee)
	s.Nil(err)
	var ts Txns
	for i := 0; i < 10; i++ {
		ts = append(ts, s.newTxn(2))
	}

	for {
		ets, err := ms.FindLeaseExpired(context.Background(), ts[0].TxnType, []string{}, 100)
		s.Nil(err)
		if len(ets) == 0 {
			break
		}
		for _, t := range ets {
			t.SetState(TxnStateAborted)
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
		ets, err := ms.FindLeaseExpired(context.Background(), ts[0].TxnType, nil, limit)
		s.Nil(err)
		idc := int64(math.MaxInt64)
		for _, t := range ets {
			s.True(compareTxnTime(t.LeaseExpireTime, expiredTime))
			s.True(t.Id < idc)
			s.True(slice.Contain(states, t.State))
			idc = t.Id
			t.SetState(TxnStateAborted)
			s.Nil(ms.Update(context.Background(), t))
		}
		if i == 1 {
			s.Equal(3, len(ets))
		}
	}
}

func (s *_Suite) TestGrantLease() {
	states := []string{TxnStatePrepared, TxnStateFailed, TxnStateCommitting}

	ms, err := NewModelStorage(s.driver, s.dsn, s.timeout, s.lessee)
	s.Nil(err)
	var ts Txns
	for i := 0; i < 10; i++ {
		ts = append(ts, s.newTxn(2))
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
			s.Equal(ErrInvalidLessee, ms.GrantLease(context.Background(), txn))
		} else {
			s.Nil(ms.GrantLease(context.Background(), txn))
		}
	}
}

func (s *_Suite) TestUpdateConditions() {

	ms, err := NewModelStorage(s.driver, s.dsn, s.timeout, s.lessee)
	s.Nil(err)

	// invalid lease
	{
		txn := s.newTxn(2)
		txn.Lessee = "test_invalid_lessee"
		s.Nil(ms.Save(context.TODO(), txn))
		txn.Branches[0].SetState(TxnStateCommitted)
		err = ms.UpdateConditions(context.Background(), txn, nil)
		s.Equal(ErrInvalidLessee, err)
	}

	txn := s.newTxn(2)
	s.Nil(ms.Save(context.TODO(), txn))

	txn.SetState(TxnStateCommitting)
	s.NotNil(ms.UpdateConditions(context.Background(), txn,
		func(oldTxn *Txn) error {
			return fmt.Errorf("test")
		}))
	gtxn, err := ms.GetByGtid(context.Background(), txn.Gtid)
	s.Nil(err)
	s.NotEqual(TxnStateCommitting, gtxn.State)

	s.Nil(ms.UpdateConditions(context.Background(), txn,
		func(oldTxn *Txn) error {
			return nil
		}))
	gtxn, err = ms.GetByGtid(context.Background(), txn.Gtid)
	s.Nil(err)
	s.Equal(TxnStateCommitting, gtxn.State)
}

func (s *_Suite) TestRegisterBranches() {

	ms, err := NewModelStorage(s.driver, s.dsn, s.timeout, s.lessee)
	s.Nil(err)

	txn := s.newTxn(2)
	var branches []*Branch
	{
		txn2 := s.newTxn(2)
		branches = txn2.Branches
		for i, branch := range branches {
			branch.Gtid = txn.Gtid
			branch.Bid = fmt.Sprintf("r_%d", i)
		}
	}

	// register expired record
	{
		txn := s.newTxn(2)
		txn.ExpireTime = time.Now().Add(-1 * time.Second)
		s.Nil(ms.Save(context.TODO(), txn))
		s.Equal(ErrInvalidLessee, ms.RegisterBranches(context.Background(), branches))
	}

	s.Nil(ms.Save(context.TODO(), txn))
	s.Nil(ms.RegisterBranches(context.Background(), branches))
}

func (s *_Suite) TestGrantLeaseIncBranch() {
	ms, err := NewModelStorage(s.driver, s.dsn, s.timeout, s.lessee)
	s.Nil(err)
	txn := s.newTxn(2)
	s.Nil(ms.Save(context.TODO(), txn))
	branch := txn.Branches[0]

	s.Nil(ms.GrantLeaseIncBranch(context.Background(), txn, branch, 120*time.Second))
	txn2, err := ms.GetByGtid(context.Background(), txn.Gtid)
	s.Nil(err)
	s.Greater(txn2.LeaseExpireTime, time.Now().Add(118*time.Second))
	s.Equal(2, txn2.Branches[0].TryCount)
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
			aa.Payload != bb.Payload || aa.Timeout != bb.Timeout ||
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

func (s *_Suite) newTxn(subCount int) *Txn {

	txn := &Txn{
		//Id:                int64(s.random.Int31()),
		Gtid:              fmt.Sprintf("test_gtid_%v", s.random.Int31()),
		Business:          "test_bz",
		State:             TxnStatePrepared,
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
			Bid:         fmt.Sprintf("%v", i),
			BranchType:  BranchTypeCommit,
			Action:      fmt.Sprintf("http://service_%v/saga", i),
			Payload:     fmt.Sprintf(`{"Gtid":%s, "Stid":%d}`, txn.Gtid, i),
			Timeout:     1 * time.Second,
			State:       TxnStatePrepared,
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
			Bid:         fmt.Sprintf("c_%v", i),
			BranchType:  BranchTypeCompensation,
			Action:      fmt.Sprintf("http://service_%v/saga/rollback", i),
			Payload:     fmt.Sprintf(`{"Gtid":%s, "Stid":%d}`, txn.Gtid, i),
			Timeout:     1 * time.Second,
			State:       TxnStatePrepared,
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
