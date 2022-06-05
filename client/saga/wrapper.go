package saga

import (
	"context"
	"time"

	"github.com/ikenchina/octopus/define"
)

type Transaction struct {
	cli     Client
	biz     string
	Request define.SagaRequest
}

// @todo support parallelly execute
func SagaTransaction(ctx context.Context, tcServer string, expire time.Time,
	branches func(t *Transaction, gtid string) error) (*define.SagaResponse, error) {

	t := &Transaction{}
	t.cli.TcServer = tcServer

	gtid, err := t.cli.NewGtid(ctx)
	if err != nil {
		return nil, err
	}

	// begin transaction
	t.Request.Gtid = gtid
	t.Request.ExpireTime = expire
	t.Request.Business = t.biz

	// transaction body
	err = branches(t, gtid)
	if err != nil {
		return nil, err
	}

	return t.cli.Commit(ctx, &t.Request)
}

func (t *Transaction) NewBranch(branchID int, commitAction string, compensationAction string, payload string) {
	t.Request.Branches = append(t.Request.Branches, define.SagaBranch{
		BranchId: branchID,
		Payload:  payload,
		Commit: define.SagaBranchCommit{
			Action:  commitAction,
			Timeout: time.Second,
			Retry: define.SagaRetry{
				MaxRetry: -1,
				Constant: &define.RetryStrategyConstant{
					Duration: time.Second,
				},
			},
		},
		Compensation: define.SagaBranchCompensation{
			Action:  compensationAction,
			Timeout: time.Second,
			Retry:   time.Second,
		},
	})
}

func (t *Transaction) SetNotify(action string, timeout, retry time.Duration) {
	t.Request.Notify = &define.SagaNotify{
		Action:  action,
		Timeout: timeout,
		Retry:   retry,
	}
}
