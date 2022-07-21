package saga

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ikenchina/octopus/common/util"
	"github.com/ikenchina/octopus/define"
	"github.com/ikenchina/octopus/define/proto/saga/pb"
	tc_rpc "github.com/ikenchina/octopus/define/proto/saga/pb"
	"github.com/ikenchina/octopus/tc/app/model"
)

const (
	minTimeout = 10 * time.Millisecond
	minRetry   = 50 * time.Millisecond
	maxPayload = 1024 * 1024
)

var (
	ErrMinTimeout        = fmt.Errorf("minimum timeout : %s", minTimeout.String())
	ErrMaxPayload        = fmt.Errorf("max payload :%d", maxPayload)
	ErrMinRetry          = fmt.Errorf("minimum retry : %s", minRetry.String())
	ErrInvalidGtid       = fmt.Errorf("invalid gtid")
	ErrInvalidNotify     = errors.New("invalid notify")
	ErrInvalidExpireTime = errors.New("invalid expire time")
	ErrInvalidBranchID   = errors.New("invalid branch id")
	ErrInvalidAction     = errors.New("invalid action")
	ErrInvalidRetry      = errors.New("invalid retry strategy")
)

func (ss *SagaService) convertToModel(saga *define.SagaRequest) (*model.Txn, error) {
	if len(saga.Gtid) == 0 {
		return nil, ErrInvalidGtid
	}
	if saga.SagaCallType != define.TxnCallTypeAsync && saga.SagaCallType != model.TxnCallTypeSync {
		return nil, errors.New("invalid call type")
	}
	if saga.SagaCallType == define.TxnCallTypeAsync {
		if saga.Notify == nil || !util.IsValidAction(saga.Notify.Action) ||
			saga.Notify.Timeout < minTimeout || saga.Notify.Retry < minRetry {
			return nil, ErrInvalidNotify
		}
	}

	now := time.Now()
	if saga.ExpireTime.Before(now) {
		return nil, ErrInvalidExpireTime
	}

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
		bs, err := ss.convertBranchToModel(&sub, saga.Gtid, now)
		if err != nil {
			return nil, err
		}
		sm.Branches = append(sm.Branches, bs...)
	}
	return sm, nil
}

func (ss *SagaService) convertBranchToModel(sub *define.SagaBranch, gtid string, now time.Time) ([]*model.Branch, error) {
	if sub.BranchId <= 0 {
		return nil, ErrInvalidBranchID
	}
	if !util.IsValidAction(sub.Commit.Action) {
		return nil, ErrInvalidAction
	}

	if (sub.Commit.Retry.Constant == nil && sub.Commit.Retry.MaxRetry > 0) ||
		(sub.Commit.Retry.Constant != nil && sub.Commit.Retry.Constant.Duration < minRetry) {
		return nil, ErrInvalidRetry
	}

	if sub.Commit.Timeout < minTimeout {
		return nil, ErrMinTimeout
	}

	if len(sub.Compensation.Action) > 0 {
		if !util.IsValidAction(sub.Compensation.Action) {
			return nil, ErrInvalidAction
		}
		if sub.Compensation.Timeout < minTimeout {
			return nil, ErrMinTimeout
		}
		if sub.Compensation.Retry < minRetry {
			return nil, ErrMinRetry
		}
	}
	if len(sub.Commit.Payload) > maxPayload || len(sub.Compensation.Payload) > maxPayload {
		return nil, ErrMaxPayload
	}

	branches := []*model.Branch{}
	do := &model.Branch{
		Gtid:        gtid,
		Bid:         sub.BranchId,
		BranchType:  model.BranchTypeCommit,
		Action:      sub.Commit.Action,
		Timeout:     sub.Commit.Timeout,
		CreatedTime: now,
		UpdatedTime: now,
		State:       model.TxnStatePrepared,
	}

	if len(sub.Commit.Payload) > 0 {
		// data, err := base64.StdEncoding.DecodeString(sub.Commit.Payload)
		// if err != nil {
		// 	return nil, err
		// }
		do.Payload = (sub.Commit.Payload)
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
		Gtid:        gtid,
		Bid:         sub.BranchId,
		BranchType:  model.BranchTypeCompensation,
		Action:      sub.Compensation.Action,
		Timeout:     sub.Compensation.Timeout,
		CreatedTime: now,
		UpdatedTime: now,
		State:       model.TxnStatePrepared,
	}
	if len(sub.Compensation.Payload) > 0 {
		// data, err := base64.StdEncoding.DecodeString(sub.Compensation.Payload)
		// if err != nil {
		// 	return nil, err
		// }
		com.Payload = (sub.Compensation.Payload)
	}

	com.Retry.MaxRetry = -1
	com.Retry.Constant = &model.RetryConstant{
		Duration: sub.Compensation.Retry,
	}
	branches = append(branches, do, com)

	return branches, nil
}

func (ss *SagaService) parseFromModel(sr *define.SagaResponse, sm *model.Txn) {
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

func (ss *SagaService) convertPbToModel(in *tc_rpc.SagaRequest) (*model.Txn, error) {
	if len(in.GetGtid()) == 0 {
		return nil, ErrInvalidGtid
	}
	if strings.ToLower(in.GetCallType().String()) == define.TxnCallTypeAsync {
		if in.GetNotify() == nil || !util.IsValidAction(in.GetNotify().GetAction()) ||
			in.GetNotify().GetTimeout().AsDuration() < minTimeout ||
			in.GetNotify().GetRetry().AsDuration() < minRetry {
			return nil, ErrInvalidNotify
		}
	}

	now := time.Now()

	if in.GetExpireTime().AsTime().Before(now) {
		return nil, ErrInvalidExpireTime
	}
	sm := &model.Txn{
		Gtid:        in.GetGtid(),
		TxnType:     model.TxnTypeSaga,
		CreatedTime: now,
		UpdatedTime: now,
		Lessee:      ss.cfg.Lessee,
		ExpireTime:  in.GetExpireTime().AsTime(),
		CallType:    strings.ToLower(in.GetCallType().String()),
		State:       model.TxnStatePrepared,
		Business:    in.GetBusiness(),
	}
	if in.GetNotify() != nil {
		sm.NotifyAction = in.GetNotify().GetAction()
		sm.NotifyTimeout = in.GetNotify().GetTimeout().AsDuration()
		sm.NotifyRetry = in.GetNotify().Retry.AsDuration()
	}

	for _, sub := range in.GetBranches() {
		bb, err := ss.convertBranchPbToModel(sub, in.Gtid, now)
		if err != nil {
			return nil, err
		}
		sm.Branches = append(sm.Branches, bb...)
	}
	return sm, nil
}

func (ss *SagaService) convertBranchPbToModel(in *tc_rpc.SagaBranchRequest, gtid string, now time.Time) ([]*model.Branch, error) {
	if in.GetBranchId() <= 0 {
		return nil, ErrInvalidBranchID
	}
	if !util.IsValidAction(in.GetCommit().GetAction()) {
		return nil, ErrInvalidAction
	}
	if (in.GetCommit().GetRetry().Strategy.GetConstant() == nil &&
		in.GetCommit().GetRetry().GetMaxRetry() > 0) ||
		in.GetCommit().GetRetry().Strategy.GetConstant() != nil &&
			in.GetCommit().GetRetry().GetStrategy().GetConstant().AsDuration() < minRetry {
		return nil, ErrInvalidRetry
	}
	if in.GetCommit().GetTimeout().AsDuration() < minTimeout {
		return nil, ErrMinTimeout
	}

	if len(in.GetCompensation().GetAction()) > 0 {
		if !util.IsValidAction(in.GetCompensation().GetAction()) {
			return nil, ErrInvalidAction
		}
		if in.GetCompensation().GetTimeout().AsDuration() < minTimeout {
			return nil, ErrMinTimeout
		}
		if in.GetCompensation().GetRetry().AsDuration() < minRetry {
			return nil, ErrMinRetry
		}
	}

	if len(in.GetPayload()) > maxPayload {
		return nil, ErrMaxPayload
	}

	branches := []*model.Branch{}
	do := &model.Branch{
		Gtid:        gtid,
		Bid:         int(in.GetBranchId()),
		BranchType:  model.BranchTypeCommit,
		Action:      in.GetCommit().GetAction(),
		Payload:     in.GetPayload(),
		Timeout:     in.GetCommit().GetTimeout().AsDuration(),
		CreatedTime: now,
		UpdatedTime: now,
		State:       model.TxnStatePrepared,
	}
	do.Retry.MaxRetry = int(in.GetCommit().GetRetry().GetMaxRetry())
	if in.GetCommit().GetRetry().GetStrategy().GetConstant() != nil {
		do.Retry.Constant = &model.RetryConstant{
			Duration: in.GetCommit().GetRetry().GetStrategy().GetConstant().AsDuration(),
		}
	}
	if do.Retry.MaxRetry < 0 {
		do.Retry.MaxRetry = math.MaxInt - 1
	}

	com := &model.Branch{
		Gtid:        gtid,
		Bid:         int(in.GetBranchId()),
		BranchType:  model.BranchTypeCompensation,
		Action:      in.GetCompensation().GetAction(),
		Timeout:     in.GetCompensation().GetTimeout().AsDuration(),
		CreatedTime: now,
		UpdatedTime: now,
		State:       model.TxnStatePrepared,
	}
	com.Retry.MaxRetry = -1
	com.Retry.Constant = &model.RetryConstant{
		Duration: in.GetCompensation().GetRetry().AsDuration(),
	}
	branches = append(branches, do, com)

	return branches, nil
}

func (ss *SagaService) parsePbFromModel(out *tc_rpc.SagaResponse, sm *model.Txn) {
	if sm == nil {
		return
	}

	if out.Saga == nil {
		out.Saga = &pb.Saga{}
	}

	out.Saga.Gtid = sm.Gtid
	out.Saga.State = sm.State

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
		out.Saga.Branches = append(out.Saga.Branches,
			&tc_rpc.SagaBranch{
				BranchId: int32(bb.Bid),
				State:    state,
				Payload:  bb.Response,
			})
	}
}
