package tcc

import (
	"errors"
	"fmt"
	"time"

	"github.com/ikenchina/octopus/common/util"
	"github.com/ikenchina/octopus/define"
	"github.com/ikenchina/octopus/define/proto/tcc/pb"
	"github.com/ikenchina/octopus/tc/app/model"
)

const (
	minTimeout = 10 * time.Millisecond
	minRetry   = 50 * time.Millisecond
	maxPayload = 1024 * 1024
)

var (
	ErrInvalidBranchID   = errors.New("invalid branch id")
	ErrInvalidGtid       = fmt.Errorf("invalid gtid")
	ErrInvalidAction     = errors.New("invalid action")
	ErrInvalidExpireTime = errors.New("invalid expire time")
	ErrMinTimeout        = fmt.Errorf("minimum timeout : %s", minTimeout.String())
	ErrMaxPayload        = fmt.Errorf("max payload :%d", maxPayload)
)

func (ss *TccService) convertToModel(tr *define.TccRequest) (*model.Txn, error) {
	if len(tr.Gtid) == 0 {
		return nil, ErrInvalidGtid
	}

	now := time.Now()
	txn := &model.Txn{
		Gtid:              tr.Gtid,
		TxnType:           model.TxnTypeTcc,
		CreatedTime:       now,
		UpdatedTime:       now,
		ExpireTime:        tr.ExpireTime,
		Lessee:            tr.Lessee,
		CallType:          model.TxnCallTypeSync,
		ParallelExecution: false,
		State:             model.TxnStatePrepared,
		Business:          tr.Business,
	}
	for _, bb := range tr.Branches {
		bb, err := ss.convertBranchToModel(&bb, tr.Gtid, now)
		if err != nil {
			return nil, err
		}
		txn.Branches = append(txn.Branches, bb...)
	}
	return txn, nil
}

func (ss *TccService) convertBranchToModel(tb *define.TccBranch, gtid string, now time.Time) ([]*model.Branch, error) {
	if tb.BranchId <= 0 {
		return nil, ErrInvalidBranchID
	}
	if !util.IsValidAction(tb.ActionConfirm) ||
		!util.IsValidAction(tb.ActionCancel) {
		return nil, ErrInvalidAction
	}
	if tb.Timeout < minTimeout {
		return nil, ErrMinTimeout
	}
	if len(tb.Payload) > maxPayload {
		return nil, ErrMaxPayload
	}

	branches := []*model.Branch{}
	branches = append(branches, &model.Branch{
		Gtid:        gtid,
		Bid:         tb.BranchId,
		BranchType:  model.BranchTypeConfirm,
		Action:      tb.ActionConfirm,
		Payload:     tb.Payload,
		Timeout:     tb.Timeout,
		CreatedTime: now,
		UpdatedTime: now,
		State:       model.TxnStatePrepared,
		Retry: model.RetryStrategy{
			Constant: &model.RetryConstant{
				Duration: tb.Retry,
			},
		},
	},
		&model.Branch{
			Gtid:        gtid,
			Bid:         tb.BranchId,
			BranchType:  model.BranchTypeCancel,
			Action:      tb.ActionCancel,
			Payload:     tb.Payload,
			Timeout:     tb.Timeout,
			CreatedTime: now,
			UpdatedTime: now,
			State:       model.TxnStatePrepared,
			Retry: model.RetryStrategy{
				Constant: &model.RetryConstant{
					Duration: tb.Retry,
				},
			},
		},
	)
	return branches, nil
}

func (ss *TccService) parseFromModel(tr *define.TccResponse, sm *model.Txn) {
	if sm == nil {
		return
	}
	tr.Gtid = sm.Gtid
	tr.State = sm.State

	bm := make(map[int]*model.Branch)
	for _, b := range sm.Branches {
		bb, ok := bm[b.Bid]
		if !ok {
			bm[b.Bid] = b
			continue
		}
		commit := bb
		rollback := b
		if bb.BranchType == model.BranchTypeCancel {
			commit = b
			rollback = bb
		}
		state := commit.State
		if rollback.State == model.TxnStateCommitted {
			state = model.TxnStateAborted
		}

		tr.Branches = append(tr.Branches, define.TccBranchResponse{
			BranchId: bb.Bid,
			State:    state,
		})
	}
}

func (ss *TccService) convertPbToModel(tcc *pb.TccRequest) (*model.Txn, error) {

	if len(tcc.GetGtid()) == 0 {
		return nil, ErrInvalidGtid
	}

	now := time.Now()
	txn := &model.Txn{
		Gtid:              tcc.GetGtid(),
		TxnType:           model.TxnTypeTcc,
		CreatedTime:       now,
		UpdatedTime:       now,
		ExpireTime:        tcc.GetExpireTime().AsTime(),
		Lessee:            ss.cfg.Lessee,
		CallType:          model.TxnCallTypeSync,
		ParallelExecution: false,
		State:             model.TxnStatePrepared,
		Business:          tcc.GetBusiness(),
	}

	for _, bb := range tcc.GetBranches() {
		bb, err := ss.convertPbBranchToModel(bb, tcc.GetGtid(), now)
		if err != nil {
			return nil, err
		}
		txn.Branches = append(txn.Branches, bb...)
	}
	return txn, nil

}

func (ss *TccService) convertPbBranchToModel(tb *pb.TccBranchRequest, gtid string, now time.Time) ([]*model.Branch, error) {

	if tb.GetBranchId() <= 0 {
		return nil, ErrInvalidBranchID
	}
	if !util.IsValidAction(tb.GetActionConfirm()) ||
		!util.IsValidAction(tb.GetActionCancel()) {
		return nil, ErrInvalidAction
	}
	if tb.GetTimeout().AsDuration() < minTimeout {
		return nil, ErrMinTimeout
	}
	if len(tb.GetPayload()) > maxPayload {
		return nil, ErrMaxPayload
	}

	branches := []*model.Branch{}
	branches = append(branches, &model.Branch{
		Gtid:        gtid,
		Bid:         int(tb.GetBranchId()),
		BranchType:  model.BranchTypeConfirm,
		Action:      tb.GetActionConfirm(),
		Payload:     tb.GetPayload(),
		Timeout:     tb.GetTimeout().AsDuration(),
		CreatedTime: now,
		UpdatedTime: now,
		State:       model.TxnStatePrepared,
		Retry: model.RetryStrategy{
			Constant: &model.RetryConstant{
				Duration: tb.GetRetry().AsDuration(),
			},
		},
	},
		&model.Branch{
			Gtid:        gtid,
			Bid:         int(tb.GetBranchId()),
			BranchType:  model.BranchTypeCancel,
			Action:      tb.GetActionCancel(),
			Timeout:     tb.GetTimeout().AsDuration(),
			CreatedTime: now,
			UpdatedTime: now,
			State:       model.TxnStatePrepared,
			Retry: model.RetryStrategy{
				Constant: &model.RetryConstant{
					Duration: tb.GetRetry().AsDuration(),
				},
			},
		},
	)
	return branches, nil
}

func (ss *TccService) parsePbFromModel(tr *pb.TccResponse, sm *model.Txn) {
	if sm == nil {
		return
	}
	if tr.Tcc == nil {
		tr.Tcc = &pb.Tcc{}
	}
	tr.Tcc.Gtid = sm.Gtid
	tr.Tcc.State = sm.State

	bm := make(map[int]*model.Branch)
	for _, b := range sm.Branches {
		bb, ok := bm[b.Bid]
		if !ok {
			bm[b.Bid] = b
			continue
		}
		commit := bb
		rollback := b
		if bb.BranchType == model.BranchTypeCancel {
			commit = b
			rollback = bb
		}
		state := commit.State
		if rollback.State == model.TxnStateCommitted {
			state = model.TxnStateAborted
		}

		tr.Tcc.Branches = append(tr.Tcc.Branches, &pb.TccBranch{
			BranchId: int32(bb.Bid),
			State:    state,
		})
	}
}
