package tcc

import (
	"errors"
	"fmt"
	"time"

	"github.com/ikenchina/octopus/common/util"
	"github.com/ikenchina/octopus/define"
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

func convertToModel(tr *define.TccRequest) *model.Txn {
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
		txn.Branches = append(txn.Branches, convertBranchToModel(&bb, tr.Gtid)...)
	}
	return txn
}

func validate(tr *define.TccRequest) error {
	if len(tr.Gtid) == 0 {
		return ErrInvalidGtid
	}

	if tr.ExpireTime.Unix() < 0 {
		return ErrInvalidExpireTime
	}

	for _, b := range tr.Branches {
		err := validateBranch(&b)
		if err != nil {
			return err
		}
	}

	return nil
}

func convertBranchToModel(tb *define.TccBranch, gtid string) (branches []*model.Branch) {

	now := time.Now()
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
	return
}

func validateBranch(tb *define.TccBranch) error {
	if tb.BranchId <= 0 {
		return ErrInvalidBranchID
	}
	if !util.IsValidAction(tb.ActionConfirm) ||
		!util.IsValidAction(tb.ActionCancel) {
		return ErrInvalidAction
	}
	if tb.Timeout < minTimeout {
		return ErrMinTimeout
	}
	if len(tb.Payload) > maxPayload {
		return ErrMaxPayload
	}
	return nil
}

func parseFromModel(tr *define.TccResponse, sm *model.Txn) {
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
