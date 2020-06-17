package saga

import (
	"errors"
	"fmt"
	"time"

	"github.com/ikenchina/octopus/app/model"
	shttp "github.com/ikenchina/octopus/common/http"
)

var (
	ErrInvalidNotify = errors.New("wrong notify")
)

type SagaRequest struct {
	NodeId            int `json:"-"`
	DcId              int `json:"-"`
	Gtid              string
	Business          string
	Notify            *SagaNotify
	ExpireTime        time.Time
	SagaCallType      string // sync or async
	Branches          []SagaBranch
	ParallelExecution bool   // parallel or serial
	Lessee            string `json:"-"`
}

const (
	minTimeout = 10 * time.Millisecond
	minRetry   = 50 * time.Millisecond
	maxPayload = 1024 * 1024
)

var (
	errMinTimeout = fmt.Errorf("minimum timeout : %s", minTimeout.String())
	errMinRetry   = fmt.Errorf("minimum retry : %s", minRetry.String())
	errMaxPayload = fmt.Errorf("max payload :%d", maxPayload)
)

func (saga *SagaRequest) Validate() error {
	if saga.SagaCallType == "" {
		saga.SagaCallType = model.TxnCallTypeSync
	}
	if saga.SagaCallType != model.TxnCallTypeAsync && saga.SagaCallType != model.TxnCallTypeSync {
		return errors.New("invalid call type")
	}
	if len(saga.Gtid) == 0 {
		return errors.New("invalid global transaction id")
	}
	if saga.SagaCallType == model.TxnCallTypeAsync {
		if saga.Notify == nil || !shttp.IsValidUrl(saga.Notify.Action) || saga.Notify.Timeout < minTimeout || saga.Notify.Retry < minRetry {
			return ErrInvalidNotify
		}
	}
	if saga.ExpireTime.Unix() <= 0 {
		saga.ExpireTime = time.Now().Add(1 * time.Hour)
	}

	for _, sub := range saga.Branches {
		err := sub.validate()
		if err != nil {
			return err
		}
	}
	return nil
}

func (saga *SagaRequest) ConvertToModel() *model.Txn {
	now := time.Now()
	sm := &model.Txn{
		Gtid:              saga.Gtid,
		TxnType:           model.TxnTypeSaga,
		CreatedTime:       now,
		UpdatedTime:       now,
		Lessee:            saga.Lessee,
		ExpireTime:        saga.ExpireTime,
		CallType:          saga.SagaCallType,
		NotifyAction:      saga.Notify.Action,
		NotifyTimeout:     saga.Notify.Timeout,
		NotifyRetry:       saga.Notify.Retry,
		ParallelExecution: saga.ParallelExecution,
		State:             model.TxnStatePrepared,
		Business:          saga.Business,
	}

	for _, sub := range saga.Branches {
		sm.Branches = append(sm.Branches, sub.ConvertToModel(saga.Gtid)...)
	}
	return sm
}

type SagaNotify struct {
	Action  string
	Timeout time.Duration
	Retry   time.Duration
}

type SagaBranchCommit struct {
	Action  string
	Timeout time.Duration
	Retry   SagaRetry
}

type SagaRetry struct {
	MaxRetry int
	Constant *RetryStrategyConstant
}

type SagaBranchCompensation struct {
	Action  string
	Timeout time.Duration
	// @todo retry strategy
	Retry time.Duration
}

type SagaBranch struct {
	BranchId     string
	Payload      string
	Commit       SagaBranchCommit
	Compensation SagaBranchCompensation
}

func (sub *SagaBranch) validate() error {
	if len(sub.BranchId) == 0 {
		return errors.New("invalid branch id")
	}
	if !shttp.IsValidUrl(sub.Commit.Action) {
		return errors.New("invalid branch url")
	}

	if (sub.Commit.Retry.Constant == nil && sub.Commit.Retry.MaxRetry > 0) ||
		(sub.Commit.Retry.Constant != nil && sub.Commit.Retry.Constant.Duration < minRetry) {
		return errors.New("invalid retry strategy")
	}

	if sub.Commit.Timeout < minTimeout {
		return errMinTimeout
	}

	if len(sub.Compensation.Action) > 0 {
		if !shttp.IsValidUrl(sub.Compensation.Action) {
			return errors.New("invalid compensation url")
		}
		if sub.Compensation.Timeout < minTimeout {
			return errMinTimeout
		}
		if sub.Compensation.Retry < minRetry {
			return errMinRetry
		}
	}
	if len(sub.Payload) > maxPayload {
		return errMaxPayload
	}

	return nil
}

func (sub *SagaBranch) ConvertToModel(gtid string) []*model.Branch {
	now := time.Now()
	branches := []*model.Branch{}

	do := &model.Branch{
		Gtid:        gtid,
		Bid:         sub.BranchId,
		BranchType:  model.BranchTypeCommit,
		Action:      sub.Commit.Action,
		Payload:     sub.Payload,
		Timeout:     sub.Commit.Timeout,
		CreatedTime: now,
		UpdatedTime: now,
		State:       model.TxnStatePrepared,
	}
	do.Retry.MaxRetry = sub.Commit.Retry.MaxRetry
	if sub.Commit.Retry.Constant != nil {
		do.Retry.Constant = &model.RetryConstant{
			Duration: sub.Commit.Retry.Constant.Duration,
		}
	}

	com := &model.Branch{
		Gtid:        gtid,
		Bid:         sub.BranchId,
		BranchType:  model.BranchTypeCompensation,
		Action:      sub.Compensation.Action,
		Payload:     sub.Payload,
		Timeout:     sub.Compensation.Timeout,
		CreatedTime: now,
		UpdatedTime: now,
		State:       model.TxnStatePrepared,
	}
	com.Retry.MaxRetry = -1
	com.Retry.Constant = &model.RetryConstant{
		Duration: sub.Compensation.Retry,
	}
	branches = append(branches, do, com)

	return branches
}

type RetryStrategyConstant struct {
	Duration time.Duration
}

type SagaResponse struct {
	Gtid     string
	State    string
	Branches []SagaBranchResponse
	Msg      string
}

func (sr *SagaResponse) ParseFromModel(sm *model.Txn) {
	sr.Gtid = sm.Gtid
	sr.State = sm.State

	bm := make(map[string]*model.Branch)
	for _, b := range sm.Branches {
		bb, ok := bm[b.Bid]
		if !ok {
			bm[b.Bid] = b
			continue
		}
		commit := bb
		rollback := b
		if bb.BranchType == model.BranchTypeCompensation {
			commit = b
			rollback = bb
		}
		state := commit.State
		if rollback.State == model.TxnStateCommitted {
			state = model.TxnStateAborted
		}
		sr.Branches = append(sr.Branches, SagaBranchResponse{
			BranchId: bb.Bid,
			State:    state,
			Payload:  bb.Response,
		})
	}
}

type SagaBranchResponse struct {
	BranchId string
	State    string
	Payload  string
}
