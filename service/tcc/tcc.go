package tcc

import (
	"errors"
	"fmt"
	"time"

	"github.com/ikenchina/octopus/app/model"
	shttp "github.com/ikenchina/octopus/common/http"
)

const (
	minTimeout = 10 * time.Millisecond
	minRetry   = 50 * time.Millisecond
	maxPayload = 1024 * 1024
)

var (
	errMinTimeout = fmt.Errorf("minimum timeout : %s", minTimeout.String())
	errMaxPayload = fmt.Errorf("max payload :%d", maxPayload)
)

type TccRequest struct {
	NodeId     int `json:"-"`
	DcId       int `json:"-"`
	Gtid       string
	Business   string
	ExpireTime time.Time
	Branches   []TccBranch
	Lessee     string `json:"-"`
}

func (tr *TccRequest) ConvertToModel() *model.Txn {
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
		txn.Branches = append(txn.Branches, bb.ConvertToModel(tr.Gtid)...)
	}
	return txn
}

func (tr *TccRequest) Validate() error {
	if len(tr.Gtid) == 0 {
		return errors.New("invalid global id")
	}

	if tr.ExpireTime.Unix() < 0 {
		tr.ExpireTime = time.Now().Add(1 * time.Hour)
	}

	for _, b := range tr.Branches {
		err := b.validate()
		if err != nil {
			return err
		}
	}

	return nil
}

type TccBranch struct {
	BranchId      string
	ActionConfirm string
	ActionCancel  string
	Payload       string
	Timeout       time.Duration
	Retry         time.Duration
}

func (tb *TccBranch) ConvertToModel(gtid string) (branches []*model.Branch) {

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

func (tb *TccBranch) validate() error {
	if len(tb.BranchId) == 0 {
		return errors.New("invalid branch id")
	}
	if !shttp.IsValidUrl(tb.ActionConfirm) ||
		!shttp.IsValidUrl(tb.ActionCancel) {
		return errors.New("invalid branch url")
	}
	if tb.Timeout < minTimeout {
		return errMinTimeout
	}
	if len(tb.Payload) > maxPayload {
		return errMaxPayload
	}
	return nil
}

type TccResponse struct {
	Gtid     string
	State    string
	Branches []TccBranchResponse
	Msg      string
}

type TccBranchResponse struct {
	BranchId string
	State    string
}

func (tr *TccResponse) ParseFromModel(sm *model.Txn) {
	tr.Gtid = sm.Gtid
	tr.State = sm.State

	bm := make(map[string]*model.Branch)
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

		tr.Branches = append(tr.Branches, TccBranchResponse{
			BranchId: bb.Bid,
			State:    state,
		})
	}
}
