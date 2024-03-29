package model

import (
	"bytes"
	"context"
	"encoding/gob"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ikenchina/octopus/common/slice"
	"github.com/ikenchina/octopus/define"
)

type modelStorageMock struct {
	sync.RWMutex
	id     int64
	lessee string

	records map[string]*Txn
	timeout time.Duration
}

func NewModelStorageMock(lessee string) ModelStorage {
	msm := &modelStorageMock{
		records: make(map[string]*Txn),
		lessee:  lessee,
	}
	return msm
}

func (store *modelStorageMock) Timeout() time.Duration {
	if store.timeout == 0 {
		return 1 * time.Second
	}
	return store.timeout
}

func (store *modelStorageMock) SetTimeout(t time.Duration) {
	store.timeout = t
}

func (store *modelStorageMock) Exist(ctx context.Context, gtid string) error {
	_, err := store.GetByGtid(ctx, gtid)
	return err
}

type Txns []*Txn

func (ts Txns) Len() int {
	return len(ts)
}
func (ts Txns) Less(i, j int) bool {
	return ts[i].Id > ts[j].Id
}
func (ts Txns) Swap(i, j int) {
	tmp := ts[i]
	ts[i] = ts[j]
	ts[j] = tmp
}

func (store *modelStorageMock) FindPreparedExpired(ctx context.Context, txnType string, limit int) ([]*Txn, error) {
	store.RLock()
	defer store.RUnlock()
	records := Txns{}

	now := time.Now()
	for _, sg := range store.records {
		if sg.State == define.TxnStatePrepared && sg.TxnType == txnType &&
			sg.ExpireTime.Before(now) {
			d := new(Txn)
			store.deepCopy(d, sg)
			records = append(records, d)
		}
	}
	sort.Sort(records)
	return records, nil
}

func (store *modelStorageMock) FindRunningLeaseExpired(ctx context.Context, txnType string, limit int) ([]*Txn, error) {
	store.RLock()
	defer store.RUnlock()
	records := Txns{}

	states := []string{define.TxnStateCommitted, define.TxnStateAborted, define.TxnStatePrepared}
	now := time.Now()
	for _, sg := range store.records {
		if !slice.Contain(states, sg.State) && sg.TxnType == txnType &&
			sg.LeaseExpireTime.Before(now) {
			d := new(Txn)
			store.deepCopy(d, sg)
			records = append(records, d)
		}
	}
	sort.Sort(records)
	return records, nil
}

func (store *modelStorageMock) GetByGtid(ctx context.Context, gtid string) (*Txn, error) {
	store.RLock()
	defer store.RUnlock()

	s, ok := store.records[gtid]
	if !ok {
		return nil, ErrNotExist
	}
	d := new(Txn)
	store.deepCopy(d, s)

	return d, nil
}

func (store *modelStorageMock) Save(ctx context.Context, txn *Txn) error {
	store.Lock()
	defer store.Unlock()

	txn.Id = atomic.AddInt64(&store.id, 1)
	for i, sub := range txn.Branches {
		sub.Id = int64(i)
	}
	d := new(Txn)
	store.deepCopy(d, txn)
	store.records[txn.Gtid] = d

	return nil
}

func (store *modelStorageMock) Update(ctx context.Context, record *Txn) error {
	store.Lock()
	defer store.Unlock()

	d := new(Txn)
	store.deepCopy(d, record)
	store.records[record.Gtid] = d
	return nil
}

func (store *modelStorageMock) deepCopy(dst, src interface{}) {
	var buf bytes.Buffer
	gob.NewEncoder(&buf).Encode(src)
	err := gob.NewDecoder(&buf).Decode(dst)
	if err != nil {
		panic(err)
	}
}

func (store *modelStorageMock) UpdateConditions(ctx context.Context, txn *Txn, cb func(oldTxn *Txn) error, returning bool) (*Txn, error) {
	store.Lock()
	defer store.Unlock()

	now := time.Now()
	for i, rr := range store.records {
		if rr.Gtid == txn.Gtid && (rr.Lessee == store.lessee ||
			rr.LeaseExpireTime.Before(now)) {
			err := cb(rr)
			if err != nil {
				return nil, err
			}
			d := new(Txn)
			store.deepCopy(d, txn)
			store.records[i] = d
			return d, nil
		}
	}
	return nil, ErrNotExist
}

func (store *modelStorageMock) UpdateStateConditions(ctx context.Context, txn *Txn, cb func(oldTxn *Txn) error) (err error) {
	store.Lock()
	defer store.Unlock()

	now := time.Now()
	for i, rr := range store.records {
		if rr.Gtid == txn.Gtid && (rr.Lessee == store.lessee ||
			rr.LeaseExpireTime.Before(now)) {
			err := cb(rr)
			if err != nil {
				return err
			}
			d := new(Txn)
			store.deepCopy(d, txn)
			store.records[i] = d
			return nil
		}
	}
	return ErrNotExist
}

func (store *modelStorageMock) GrantLease(ctx context.Context, txn *Txn, lease time.Duration) error {
	store.Lock()
	defer store.Unlock()

	now := time.Now()
	for _, rr := range store.records {
		if rr.Gtid == txn.Gtid && (rr.Lessee == store.lessee || rr.LeaseExpireTime.Before(now)) {
			rr.Lessee = txn.Lessee
			rr.LeaseExpireTime = now.Add(lease)
			rr.UpdatedTime = now
			return nil
		}
	}
	return ErrNotExist
}

func (store *modelStorageMock) GrantLeaseIncBranch(ctx context.Context, txn *Txn, branch *Branch, leaseDuration time.Duration) error {
	store.Lock()
	defer store.Unlock()

	now := time.Now()
	for _, rr := range store.records {
		if rr.Gtid == txn.Gtid && (rr.Lessee == store.lessee || rr.LeaseExpireTime.Before(now)) {
			rr.Lessee = txn.Lessee
			rr.LeaseExpireTime = time.Now().Add(leaseDuration)
			rr.UpdatedTime = now
			for _, bb := range rr.Branches {
				if branch.Bid == bb.Bid {
					bb.TryCount++
					break
				}
			}
			return nil
		}
	}
	return ErrNotExist
}

func (store *modelStorageMock) GrantLeaseIncBranchCheckState(ctx context.Context, txn *Txn, branch *Branch,
	leaseDuration time.Duration, states []string) (err error) {

	store.Lock()
	defer store.Unlock()

	now := time.Now()
	for _, rr := range store.records {
		if rr.Gtid == txn.Gtid && (rr.Lessee == store.lessee || rr.LeaseExpireTime.Before(now)) {
			rr.Lessee = txn.Lessee
			rr.LeaseExpireTime = time.Now().Add(leaseDuration)
			rr.UpdatedTime = now
			for _, bb := range rr.Branches {
				if branch.Bid == bb.Bid && slice.Contain(states, bb.State) {
					bb.TryCount++
					break
				}
			}
			return nil
		}
	}
	return ErrNotExist

}

func (store *modelStorageMock) UpdateBranch(ctx context.Context, sub *Branch) error {

	rr, err := store.GetByGtid(ctx, sub.Gtid)
	if err != nil {
		return err
	}

	now := time.Now()
	if rr.Lessee != store.lessee && rr.LeaseExpireTime.After(now) {
		return ErrInvalidLessee
	}

	for _, subTxn := range rr.Branches {
		if subTxn.Bid == sub.Bid {
			d := new(Branch)
			store.deepCopy(d, sub)
			subTxn = d
			break
		}
	}

	return store.Save(ctx, rr)
}

func (store *modelStorageMock) RegisterBranches(ctx context.Context, branches []*Branch) error {

	if len(branches) == 0 {
		return nil
	}

	store.Lock()
	defer store.Unlock()

	for _, rr := range store.records {
		if rr.Gtid == branches[0].Gtid {
			d := []*Branch{}
			for _, bb := range branches {
				b := &Branch{}
				store.deepCopy(b, bb)
				d = append(d, b)
			}
			rr.Branches = append(rr.Branches, d...)
		}
	}
	return nil
}

func (store *modelStorageMock) CleanExpiredTxns(ctx context.Context, txnType string, untilTime time.Time, limit int) ([]*Txn, error) {
	store.Lock()
	defer store.Unlock()

	states := []string{define.TxnStateAborted, define.TxnStateCommitted}
	txns := make([]*Txn, 0)
	for _, rr := range store.records {
		if rr.ExpireTime.Before(untilTime) {
			if len(txns) >= limit {
				break
			}
			if slice.Contain(states, rr.State) {
				txns = append(txns, rr)
			}
		}
	}

	for _, t := range txns {
		delete(store.records, t.Gtid)
	}
	return txns, nil
}
