package saga

import (
	"context"
	"time"

	"github.com/ikenchina/octopus/define"
	"google.golang.org/protobuf/proto"
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

// grpc branch
func (t *Transaction) NewGrpcBranch(branchID int, grpcServer string, commitAction string, compensationAction string, payload proto.Message) error {
	data, err := proto.Marshal(payload)
	if err != nil {
		return err
	}

	commitAction = define.RmProtocolGrpc + define.RmProtocolSeparate + define.RmProtocolSeparate + grpcServer + commitAction
	compensationAction = define.RmProtocolGrpc + define.RmProtocolSeparate + define.RmProtocolSeparate + grpcServer + compensationAction

	t.Request.Branches = append(t.Request.Branches, define.SagaBranch{
		BranchId: branchID,
		Payload:  string(data),
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
	return nil
}

// http branch
func (t *Transaction) NewHttpBranch(branchID int, commitAction string, compensationAction string, payload string) {
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
