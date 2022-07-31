package tcc

import (
	"context"
	"strconv"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/ikenchina/octopus/define/proto/tcc/pb"
	tcc_pb "github.com/ikenchina/octopus/define/proto/tcc/pb"
	"github.com/ikenchina/octopus/tc/app/executor"
	"github.com/ikenchina/octopus/tc/app/model"
)

func (ss *TccService) NewGtid(ctx context.Context, in *empty.Empty) (resp *tcc_pb.TccResponse, err error) {
	defer requestTimer.Timer()("grpc", "NewGtid", status.Code(err).String())

	id, err := ss.idGenerator.NextId()
	if err != nil {
		return nil, err
	}

	return &tcc_pb.TccResponse{
		Tcc: &tcc_pb.Tcc{
			Gtid: strconv.FormatInt(id, 10),
		},
	}, nil
}

func (ss *TccService) Get(ctx context.Context, in *tcc_pb.TccRequest) (resp *tcc_pb.TccResponse, err error) {
	defer requestTimer.Timer()("grpc", "Get", status.Code(err).String())

	if ss.closed() {
		return nil, status.Error(codes.Unavailable, "closed")
	}
	ss.wait.Add(1)
	defer ss.wait.Done()

	gtid := in.GetGtid()
	txn, err := ss.executor.Get(ctx, gtid)
	if err != nil {
		return nil, status.Error(ss.toGrpcStatusCode(err), err.Error())
	}

	resp = &tcc_pb.TccResponse{}
	ss.parsePbFromModel(resp, txn)
	return resp, nil
}

func (ss *TccService) Prepare(ctx context.Context, in *tcc_pb.TccRequest) (resp *tcc_pb.TccResponse, err error) {
	defer requestTimer.Timer()("grpc", "Prepare", status.Code(err).String())

	if ss.closed() {
		return nil, status.Error(codes.Unavailable, "closed")
	}
	ss.wait.Add(1)
	defer ss.wait.Done()

	task, err := ss.convertPbToModel(in)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	future := ss.executor.Prepare(ctx, task)
	err = future.GetError()
	if err != nil {
		return nil, status.Error(ss.toGrpcStatusCode(err), err.Error())
	}

	resp = &pb.TccResponse{}
	ss.parsePbFromModel(resp, future.Txn)
	return resp, nil
}

func (ss *TccService) Register(ctx context.Context, in *tcc_pb.TccRequest) (resp *tcc_pb.TccResponse, err error) {
	defer requestTimer.Timer()("grpc", "Register", status.Code(err).String())

	if ss.closed() {
		return nil, status.Error(codes.Unavailable, "closed")
	}
	ss.wait.Add(1)
	defer ss.wait.Done()

	task, err := ss.convertPbToModel(in)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	future := ss.executor.Register(ctx, task)
	err = future.GetError()
	if err != nil {
		return nil, status.Error(ss.toGrpcStatusCode(err), err.Error())
	}

	resp = &pb.TccResponse{}
	ss.parsePbFromModel(resp, future.Txn)
	return resp, nil
}

func (ss *TccService) Confirm(ctx context.Context, in *tcc_pb.TccRequest) (resp *tcc_pb.TccResponse, err error) {
	defer requestTimer.Timer()("grpc", "Confirm", status.Code(err).String())

	if ss.closed() {
		return nil, status.Error(codes.Unavailable, "closed")
	}
	ss.wait.Add(1)
	defer ss.wait.Done()

	gtid := in.GetGtid()
	future := ss.executor.Commit(ctx, &model.Txn{
		Gtid: gtid,
	})
	err = future.GetError()
	if err != nil {
		return nil, status.Error(ss.toGrpcStatusCode(err), err.Error())
	}

	resp = &pb.TccResponse{}
	ss.parsePbFromModel(resp, future.Txn)
	return resp, nil
}

func (ss *TccService) Cancel(ctx context.Context, in *tcc_pb.TccRequest) (resp *tcc_pb.TccResponse, err error) {
	defer requestTimer.Timer()("grpc", "Cancel", status.Code(err).String())

	if ss.closed() {
		return nil, status.Error(codes.Unavailable, "closed")
	}
	ss.wait.Add(1)
	defer ss.wait.Done()

	gtid := in.GetGtid()
	future := ss.executor.Rollback(ctx, &model.Txn{
		Gtid: gtid,
	})

	err = future.GetError()
	if err != nil {
		return nil, status.Error(ss.toGrpcStatusCode(err), err.Error())
	}

	resp = &pb.TccResponse{}
	ss.parsePbFromModel(resp, future.Txn)
	return resp, nil
}

func (ss *TccService) toGrpcStatusCode(err error) codes.Code {
	if err == nil {
		return codes.OK
	}
	switch err {
	case executor.ErrTimeout:
		return codes.DeadlineExceeded
	case executor.ErrCancel:
		return codes.Canceled
	case executor.ErrTaskAlreadyExist, executor.ErrInProgress:
		return codes.AlreadyExists
	case executor.ErrNotExists:
		return codes.NotFound
	case executor.ErrStateIsAborted, executor.ErrStateIsCommitted:
		return codes.FailedPrecondition
	case executor.ErrExecutorClosed:
		return codes.Unavailable
	default:
		return codes.Internal
	}
}
