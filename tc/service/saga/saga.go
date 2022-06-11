package saga

import (
	"errors"
	"fmt"
	"math"
	"time"

	shttp "github.com/ikenchina/octopus/common/http"
	"github.com/ikenchina/octopus/define"
	"github.com/ikenchina/octopus/tc/app/model"
)

const (
	minTimeout = 10 * time.Millisecond
	minRetry   = 50 * time.Millisecond
	maxPayload = 1024 * 1024
)

var (
	errMinTimeout = fmt.Errorf("minimum timeout : %s", minTimeout.String())
	errMaxPayload = fmt.Errorf("max payload :%d", maxPayload)
	errMinRetry   = fmt.Errorf("minimum retry : %s", minRetry.String())

	ErrInvalidNotify = errors.New("wrong notify")
)

func Validate(saga *define.SagaRequest) error {
	if saga.SagaCallType == "" {
		saga.SagaCallType = define.TxnCallTypeSync
	}
	if saga.SagaCallType != define.TxnCallTypeAsync && saga.SagaCallType != model.TxnCallTypeSync {
		return errors.New("invalid call type")
	}
	if len(saga.Gtid) == 0 {
		return errors.New("invalid global transaction id")
	}
	if saga.SagaCallType == define.TxnCallTypeAsync {
		if saga.Notify == nil || !shttp.IsValidUrl(saga.Notify.Action) || saga.Notify.Timeout < minTimeout || saga.Notify.Retry < minRetry {
			return ErrInvalidNotify
		}
	}
	if saga.ExpireTime.Unix() <= 0 {
		saga.ExpireTime = time.Now().Add(1 * time.Hour)
	}

	for _, sub := range saga.Branches {
		err := validate(&sub)
		if err != nil {
			return err
		}
	}
	return nil
}

func convertToModel(saga *define.SagaRequest) *model.Txn {
	now := time.Now()
	sm := &model.Txn{
		Gtid:              saga.Gtid,
		TxnType:           model.TxnTypeSaga,
		CreatedTime:       now,
		UpdatedTime:       now,
		Lessee:            saga.Lessee,
		ExpireTime:        saga.ExpireTime,
		CallType:          saga.SagaCallType,
		ParallelExecution: saga.ParallelExecution,
		State:             model.TxnStatePrepared,
		Business:          saga.Business,
	}
	if saga.Notify != nil {
		sm.NotifyAction = saga.Notify.Action
		sm.NotifyTimeout = saga.Notify.Timeout
		sm.NotifyRetry = saga.Notify.Retry
	}

	for _, sub := range saga.Branches {
		sm.Branches = append(sm.Branches, convertBranchToModel(&sub, saga.Gtid)...)
	}
	return sm
}

func validate(sub *define.SagaBranch) error {
	if sub.BranchId <= 0 {
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

func convertBranchToModel(sub *define.SagaBranch, gtid string) []*model.Branch {
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
	if do.Retry.MaxRetry < 0 {
		do.Retry.MaxRetry = math.MaxInt - 1
	}

	com := &model.Branch{
		Gtid:       gtid,
		Bid:        sub.BranchId,
		BranchType: model.BranchTypeCompensation,
		Action:     sub.Compensation.Action,
		//Payload:     sub.Payload,
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

func parseFromModel(sr *define.SagaResponse, sm *model.Txn) {
	sr.Gtid = sm.Gtid
	sr.State = sm.State

	bm := make(map[int]*model.Branch)
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
		sr.Branches = append(sr.Branches, define.SagaBranchResponse{
			BranchId: bb.Bid,
			State:    state,
			Payload:  bb.Response,
		})
	}
}
