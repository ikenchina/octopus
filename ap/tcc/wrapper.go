package tcc

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	sgrpc "github.com/ikenchina/octopus/common/grpc"
	shttp "github.com/ikenchina/octopus/common/http"
	"github.com/ikenchina/octopus/define"
	"github.com/ikenchina/octopus/define/proto/tcc/pb"
)

// Transaction
// A distributed TCC transaction
type Transaction struct {
	biz  string
	gtid string
	ctx  context.Context
	cli  *GrpcClient
}

// TccTransaction start a TCC transaction as a block,
//   return TccResponse and error,
//   transaction is aborted if error is not nil,
//   TccResponse describes states of TCC transaction
func TccTransaction(ctx context.Context, cli *GrpcClient, expire time.Time,
	tryFunctions func(ctx context.Context, t *Transaction, gtid string) error) (*pb.TccResponse, error) {
	t := &Transaction{
		ctx: ctx,
		cli: cli,
	}
	newGtid, err := cli.NewGtid(ctx, &empty.Empty{})
	if err != nil {
		return nil, err
	}
	gtid := newGtid.GetTcc().GetGtid()
	t.gtid = gtid

	// begin transaction
	_, err = cli.Prepare(ctx, &pb.TccRequest{
		Gtid:       gtid,
		Business:   t.biz,
		ExpireTime: timestamppb.New(expire),
	})
	if err != nil {
		return nil, err
	}

	cancel := func(err error) (*pb.TccResponse, error) {
		resp, err2 := cli.Cancel(ctx, &pb.TccRequest{
			Gtid: gtid,
		})
		if err2 == nil {
			return resp, err
		}
		return nil, fmt.Errorf("%v, %v", err, err2)
	}
	confirm := func() (*pb.TccResponse, error) {
		return cli.Confirm(ctx, &pb.TccRequest{
			Gtid: gtid,
		})
	}

	// transaction body
	err = tryFunctions(ctx, t, gtid)
	if err != nil {
		// rollback
		return cancel(err)
	}
	// commit
	return confirm()
}

// TryGrpc invoke try method of grpc server of RM to start a branch transaction.
//   branchID is unique identifier, ensure it is unique in a TCC transaction
//   conn is grpc client connection of RM
//   try is try method of grpc server
//   confirm is confirm method of grpc server
//   cancel is cancel method of grpc server
//   request is request structure of try, confirm and cancel method
//   response is response of try method
//   opts set options of branch transaction
func (t *Transaction) TryGrpc(branchID int, conn *grpc.ClientConn,
	try string, confirm string, cancel string,
	request proto.Message,
	response interface{},
	opts ...func(o *options)) (err error) {

	rmServer := conn.Target()
	confirmAction := define.RmProtocolGrpc + define.RmProtocolSeparate +
		rmServer + define.RmProtocolSeparate + confirm
	cancelAction := define.RmProtocolGrpc + define.RmProtocolSeparate +
		rmServer + define.RmProtocolSeparate + cancel

	payload := []byte{}
	if request != nil {
		payload, err = proto.Marshal(request)
		if err != nil {
			return err
		}
	}

	return t.try(branchID,
		confirmAction,
		cancelAction,
		payload,
		func(ctx context.Context) error {
			return conn.Invoke(ctx, try, request, response)
		}, opts...)
}

func (t *Transaction) try(branchID int,
	confirm string, cancel string,
	payload []byte,
	tryf func(ctx context.Context) error,
	opts ...func(o *options)) error {

	opt := options{
		timeout: time.Second * 2,
		retry:   time.Second * 2,
	}
	for _, o := range opts {
		o(&opt)
	}

	_, err := t.cli.Register(t.ctx, &pb.TccRequest{
		Gtid: t.gtid,
		Branches: []*pb.TccBranchRequest{
			{
				BranchId:      int32(branchID),
				ActionConfirm: confirm,
				ActionCancel:  cancel,
				Payload:       payload,
				Timeout:       durationpb.New(opt.timeout),
				Retry:         durationpb.New(opt.retry),
			},
		},
	})
	if err != nil {
		return err
	}
	ctx := sgrpc.SetMetaFromOutgoingContext(t.ctx, t.gtid, branchID)
	return tryf(ctx)
}

// TryHttp invoke try method of http server of RM to start a branch transaction.
//   branchID is unique identifier, ensure it is unique in a TCC transaction
//   try is try URL of http server
//   confirm is confirm URL of http server
//   cancel is cancel URL of http server
//   request is request structure of try, confirm and cancel
//   response is response of try
//   opts set options of branch transaction
func (t *Transaction) TryHttp(branchID int,
	try string, confirm string, cancel string,
	payload []byte,
	opts ...func(o *options)) ([]byte, error) {

	resp := []byte{}
	err := t.try(branchID, confirm, cancel, payload,
		func(ctx context.Context) error {
			body, code, err := shttp.Post(t.ctx, t.gtid, try, payload)
			if err != nil {
				return err
			}
			if code >= http.StatusBadRequest {
				return fmt.Errorf("status code : %d", code)
			}
			resp = body
			return nil
		})
	return resp, err
}
