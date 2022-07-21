package model

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	TxnTypeSaga = "saga"
	TxnTypeTcc  = "tcc"

	BranchTypeCommit       = "commit"
	BranchTypeCompensation = "compensation"
	BranchTypeConfirm      = "confirm"
	BranchTypeCancel       = "cancel"

	//
	TxnStatePrepared = "prepared"

	TxnStateFailed  = "failed"
	TxnStateAborted = "aborted"

	TxnStateCommitting = "committing"
	TxnStateCommitted  = "committed"

	TxnCallTypeSync  = "sync"
	TxnCallTypeAsync = "async"
)

type Txn struct {
	Id                int64
	Gtid              string
	Business          string
	State             string
	TxnType           string
	UpdatedTime       time.Time
	CreatedTime       time.Time
	ExpireTime        time.Time
	LeaseExpireTime   time.Time
	Lessee            string
	CallType          string
	NotifyAction      string
	NotifyTimeout     time.Duration
	NotifyRetry       time.Duration
	NotifyCount       int
	Branches          []*Branch `gorm:"-" json:"branches,omitempty"`
	ParallelExecution bool
	updateFields      map[string]struct{} `gorm:"-" json:"-"`
}

func (s *Txn) NeedNotify() bool {
	return len(s.NotifyAction) > 0
}

func (s *Txn) BeginSave() {
	s.addUpdateField("updated_time")
	s.UpdatedTime = time.Now()
}

func (s *Txn) EndSave() {
	s.updateFields = make(map[string]struct{}, 0)
	for _, ss := range s.Branches {
		ss.EndSave()
	}
}

func (s *Txn) getUpdateFields() []string {
	uf := []string{}
	for k := range s.updateFields {
		uf = append(uf, k)
	}
	return uf
}

func (s *Txn) addUpdateField(uc string) {
	if s.updateFields == nil {
		s.updateFields = make(map[string]struct{})
		for _, sub := range s.Branches {
			if sub.updateFields == nil {
				sub.updateFields = make(map[string]struct{})
			}
		}
	}
	s.updateFields[uc] = struct{}{}
}

func (s *Txn) SetState(state string) {
	if s.State == state {
		return
	}
	s.addUpdateField("state")
	s.State = state
}

func (s *Txn) SetLessee(l string) {
	if s.Lessee == l {
		return
	}
	s.addUpdateField("lessee")
	s.Lessee = l
}

func (s *Txn) IncrNotify() {
	s.NotifyCount++
	s.addUpdateField("notify_count")
}

func (s *Txn) SetLeaseExpireTime(t time.Time) {
	if s.LeaseExpireTime == t {
		return
	}
	s.addUpdateField("lease_expire_time")
	s.LeaseExpireTime = t
}

func (*Txn) TableName() string {
	return "dtx.global_txn"
}

type Branch struct {
	Id           int64
	Gtid         string
	Bid          int
	BranchType   string
	Action       string
	Payload      []byte
	Timeout      time.Duration
	Response     []byte
	Retry        RetryStrategy
	Lease        time.Time `gorm:"-"`
	TryCount     int
	State        string
	UpdatedTime  time.Time
	CreatedTime  time.Time
	updateFields map[string]struct{} `gorm:"-" json:"-"`
}

func (*Branch) TableName() string {
	return "dtx.branch_action"
}

func (s *Branch) getUpdateFields() []string {
	uf := []string{}
	for k := range s.updateFields {
		uf = append(uf, k)
	}
	return uf
}

func (s *Branch) addUpdateField(us string) {
	if s.updateFields == nil {
		s.updateFields = make(map[string]struct{})
	}
	s.updateFields[us] = struct{}{}
}

func (s *Branch) EndSave() {
	s.updateFields = make(map[string]struct{})
}

func (s *Branch) CanTry() bool {
	return s.TryCount < (s.Retry.MaxRetry + 1)
}

func (b *Branch) RetryDuration() time.Duration {
	if b.Retry.Constant == nil {
		return time.Duration(0)
	}
	return b.Retry.Constant.Duration
}

func (s *Branch) BeginSave() {
	s.addUpdateField("updated_time")
	s.UpdatedTime = time.Now()
}

func (s *Branch) SetState(state string) {
	if state == s.State {
		return
	}
	s.addUpdateField("state")
	s.State = state
}

func (s *Branch) SetResponse(resp []byte) {
	if bytes.Equal(resp, s.Response) {
		return
	}
	s.addUpdateField("response")
	s.Response = resp
}

func (s *Branch) IncrTryCount() {
	s.addUpdateField("try_count")
	s.TryCount++
}

func (s *Branch) SetLease(t time.Time) {
	s.Lease = t
}

type RetryConstant struct {
	Duration time.Duration
}

type RetryStrategy struct {
	MaxRetry int
	Constant *RetryConstant
}

func (rs *RetryStrategy) Scan(value interface{}) error {
	bytes, ok := value.(string)
	if !ok {
		return errors.New(fmt.Sprint("Failed to unmarshal JSON value:", value))
	}

	result := RetryStrategy{}
	err := json.Unmarshal([]byte(bytes), &result)
	*rs = RetryStrategy(result)
	return err
}

func (rs RetryStrategy) Value() (driver.Value, error) {
	data, err := json.Marshal(&rs)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data).MarshalJSON()
}
