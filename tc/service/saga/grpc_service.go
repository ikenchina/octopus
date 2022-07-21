package saga

import (
	"context"
	"strconv"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/ikenchina/octopus/define"
	tc_rpc "github.com/ikenchina/octopus/define/proto/saga/pb"
	executor "github.com/ikenchina/octopus/tc/app/executor"
	"github.com/ikenchina/octopus/tc/app/model"
)

func (ss *SagaService) NewGtid(ctx context.Context, in *empty.Empty) (*tc_rpc.SagaResponse, error) {
	id, err := ss.idGenerator.NextId()
	if err != nil {
		return nil, err
	}

	return &tc_rpc.SagaResponse{
		Saga: &tc_rpc.Saga{
			Gtid: strconv.FormatInt(id, 10),
		},
	}, nil
}

func (ss *SagaService) Commit(ctx context.Context, in *tc_rpc.SagaRequest) (*tc_rpc.SagaResponse, error) {
	if ss.closed() {
		return nil, status.Error(codes.Unavailable, "closed")
	}

	ss.wait.Add(1)
	defer ss.wait.Done()

	saga, err := ss.parsePbSaga(in)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if saga.CallType == define.TxnCallTypeAsync {
		ctx = context.Background()
	}
	future := ss.executor.Commit(ctx, saga)

	if saga.CallType == define.TxnCallTypeSync {
		err = future.GetError()
	} else {
		select {
		case err = <-future.Get():
		default:
		}
	}
	if err != nil {
		return nil, status.Error(ss.toGrpcStatusCode(err), err.Error())
	}

	resp := &tc_rpc.SagaResponse{}
	ss.parsePbFromModel(resp, future.Txn)
	return resp, nil
}

func (ss *SagaService) Get(ctx context.Context, in *tc_rpc.SagaRequest) (*tc_rpc.SagaResponse, error) {
	if ss.closed() {
		return nil, status.Error(codes.Unavailable, "closed")
	}

	ss.wait.Add(1)
	defer ss.wait.Done()

	gtid := in.GetGtid()
	saga, err := ss.executor.Get(context.Background(), gtid)
	if err != nil {
		return nil, status.Error(ss.toGrpcStatusCode(err), err.Error())
	}

	resp := &tc_rpc.SagaResponse{}
	ss.parsePbFromModel(resp, saga)
	return resp, nil
}

func (ss *SagaService) parsePbSaga(in *tc_rpc.SagaRequest) (*model.Txn, error) {
	return ss.convertPbToModel(in)
}

func (ss *SagaService) toGrpcStatusCode(err error) codes.Code {
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
