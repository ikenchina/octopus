package saga

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"github.com/ikenchina/octopus/define"
	pb "github.com/ikenchina/octopus/define/proto/saga/pb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Transaction
// A distributed SAGA transaction
type Transaction struct {
	biz     string
	Request pb.SagaRequest
	err     error
}

// SagaTransaction start a transaction as a block,
//   return SagaResponse and error, transaction rolled back if it is not nil,
//   SagaResponse describes states of saga transaction
func SagaTransaction(ctx context.Context, cli *GrpcClient, expire time.Time,
	branches func(t *Transaction, gtid string) error) (*pb.SagaResponse, error) {

	t := &Transaction{}
	newGtid, err := cli.NewGtid(ctx, &empty.Empty{})
	if err != nil {
		return nil, err
	}
	gtid := newGtid.GetSaga().GetGtid()

	// begin transaction
	t.Request.Gtid = gtid
	t.Request.ExpireTime = timestamppb.New(expire)
	t.Request.Business = t.biz
	t.Request.CallType = pb.SagaRequest_SYNC

	// transaction body
	err = branches(t, gtid)
	if err != nil {
		return nil, err
	}
	if t.err != nil {
		return nil, t.err
	}
	return cli.Commit(ctx, &t.Request)
}

/*
func SagaTransactionHttp(ctx context.Context, cli *HttpClient, expire time.Time,
	branches func(t *Transaction, gtid string) error) (*define.SagaResponse, error) {

	t := &Transaction{}


	gtid, err := cli.NewGtid(ctx)
	if err != nil {
		return nil, err
	}

	// begin transaction
	t.Request.Gtid = gtid
	t.Request.ExpireTime = expire
	t.Request.Business = t.biz
	t.Request.SagaCallType = define.TxnCallTypeSync

	// transaction body
	err = branches(t, gtid)
	if err != nil {
		return nil, err
	}
	if t.err != nil {
		return nil, t.err
	}
	return t.cli.Commit(ctx, &t.Request)
}
*/

type branchOptions struct {
	maxRetry   int
	timeout    time.Duration
	constRetry time.Duration
}

func defaultBranchOptions() *branchOptions {
	return &branchOptions{
		maxRetry:   -1,
		timeout:    time.Second * 3,
		constRetry: time.Second * 3,
	}
}

type branchFunctions func(o *branchOptions)

// WithMaxRetry set max retry times
func WithMaxRetry(times int) branchFunctions {
	return func(o *branchOptions) {
		o.maxRetry = times
	}
}

// WithTimeout set timeout for branch transaction
func WithTimeout(to time.Duration) branchFunctions {
	return func(o *branchOptions) {
		o.timeout = to
	}
}

// WithConstantRetry set constant retry duration
func WithConstantRetry(dur time.Duration) branchFunctions {
	return func(o *branchOptions) {
		o.constRetry = dur
	}
}

// AddGrpcBranch create a branch transaction for RM(resource manager) which is a grpc server
//  branchID is unique identifier, ensure it is unique in a saga transaction
//  rmServer is grpc target of RM,
//  commitAction is grpc method to commit branch transaction
//  compensationAction is grpc method to compensate branch transaction
//  payload is request of grpc method to commit or compensate
func (t *Transaction) AddGrpcBranch(branchID int, rmServer string,
	commitAction string, compensationAction string,
	payload proto.Message,
	opts ...branchFunctions) {

	bOpts := defaultBranchOptions()
	for _, o := range opts {
		o(bOpts)
	}

	commitAction = define.RmProtocolGrpc + define.RmProtocolSeparate +
		rmServer + define.RmProtocolSeparate + commitAction
	compensationAction = define.RmProtocolGrpc + define.RmProtocolSeparate +
		rmServer + define.RmProtocolSeparate + compensationAction

	branch := &pb.SagaBranchRequest{
		BranchId: int32(branchID),
		Commit: &pb.SagaBranchRequest_Commit{
			Action:  commitAction,
			Timeout: durationpb.New(bOpts.timeout),
			Retry: &pb.SagaBranchRequest_Retry{
				MaxRetry: int32(bOpts.maxRetry),
				Strategy: &pb.SagaBranchRequest_RetryStrategy{
					Strategy: &pb.SagaBranchRequest_RetryStrategy_Constant{
						Constant: durationpb.New(bOpts.constRetry),
					},
				},
			},
		},
		Compensation: &pb.SagaBranchRequest_Compensation{
			Action:  compensationAction,
			Timeout: durationpb.New(bOpts.timeout),
			Retry:   durationpb.New(bOpts.constRetry),
		},
	}

	if payload != nil {
		data, err := proto.Marshal(payload)
		if err != nil {
			t.err = err
			return
		}
		branch.Payload = data
	}
	t.Request.Branches = append(t.Request.Branches, branch)
}

// AddHttpBranch create a branch transaction for RM(resource manager) which is a http server
//  branchID is unique identifier, ensure it is unique in a saga transaction
//  commitAction is URL to commit branch transaction with http POST method
//  compensationAction is URL to compensate branch transaction with http DELETE method
//  payload is http request body for commit or compensate actions
func (t *Transaction) AddHttpBranch(branchID int,
	commitAction string, compensationAction string,
	payload []byte,
	opts ...branchFunctions) {

	bOpts := defaultBranchOptions()
	for _, o := range opts {
		o(bOpts)
	}

	branch := &pb.SagaBranchRequest{
		BranchId: int32(branchID),
		Payload:  payload,
		Commit: &pb.SagaBranchRequest_Commit{
			Action:  commitAction,
			Timeout: durationpb.New(bOpts.timeout),
			Retry: &pb.SagaBranchRequest_Retry{
				MaxRetry: int32(bOpts.maxRetry),
				Strategy: &pb.SagaBranchRequest_RetryStrategy{
					Strategy: &pb.SagaBranchRequest_RetryStrategy_Constant{
						Constant: durationpb.New(bOpts.constRetry),
					},
				},
			},
		},
		Compensation: &pb.SagaBranchRequest_Compensation{
			Action:  compensationAction,
			Timeout: durationpb.New(bOpts.timeout),
			Retry:   durationpb.New(bOpts.constRetry),
		},
	}

	t.Request.Branches = append(t.Request.Branches, branch)
}

func (t *Transaction) SetHttpNotify(action string, timeout, retry time.Duration) {
	t.Request.Notify = &pb.SagaNotify{
		Action:  action,
		Timeout: durationpb.New(timeout),
		Retry:   durationpb.New(retry),
	}
}

func (t *Transaction) SetGrpcNotify(grpcServer string, action string, timeout, retry time.Duration) {
	na := define.RmProtocolGrpc + define.RmProtocolSeparate +
		grpcServer + define.RmProtocolSeparate + action
	t.Request.Notify = &pb.SagaNotify{
		Action:  na,
		Timeout: durationpb.New(timeout),
		Retry:   durationpb.New(retry),
	}
}

func (t *Transaction) Abort() {

}
