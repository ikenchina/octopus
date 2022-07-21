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

type Transaction struct {
	biz     string
	Request pb.SagaRequest
	err     error
}

// @todo Cancel interface

// @todo support parallelly execute
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

func WithMaxRetry(times int) branchFunctions {
	return func(o *branchOptions) {
		o.maxRetry = times
	}
}

func WithTimeout(to time.Duration) branchFunctions {
	return func(o *branchOptions) {
		o.timeout = to
	}
}

func WithConstantRetry(dur time.Duration) branchFunctions {
	return func(o *branchOptions) {
		o.constRetry = dur
	}
}

// grpc branch
func (t *Transaction) NewGrpcBranch(branchID int, grpcServer string,
	commitAction string, compensationAction string,
	payload proto.Message,
	opts ...branchFunctions) {

	bOpts := defaultBranchOptions()
	for _, o := range opts {
		o(bOpts)
	}

	commitAction = define.RmProtocolGrpc + define.RmProtocolSeparate +
		grpcServer + define.RmProtocolSeparate + commitAction
	compensationAction = define.RmProtocolGrpc + define.RmProtocolSeparate +
		grpcServer + define.RmProtocolSeparate + compensationAction

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

func (t *Transaction) NewHttpBranch(branchID int,
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
