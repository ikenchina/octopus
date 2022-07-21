package saga

import (
	"context"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	cgrpc "github.com/ikenchina/octopus/common/grpc"
	logutil "github.com/ikenchina/octopus/common/log"
	sagarm "github.com/ikenchina/octopus/rm/saga"
	"github.com/ikenchina/octopus/test/rm/saga/pb"
)

type SagaRmBankGrpcService struct {
	pb.UnsafeSagaBankServiceServer
	listen string
	db     *gorm.DB
}

func NewSagaRmBankGrpcService(listen string, db *gorm.DB) *SagaRmBankGrpcService {
	return &SagaRmBankGrpcService{
		listen: listen,
		db:     db,
	}
}

func (rm *SagaRmBankGrpcService) In(ctx context.Context, in *pb.SagaRequest) (*pb.SagaResponse, error) {
	gtid, bid := cgrpc.ParseContextMeta(ctx)
	resp := &pb.SagaResponse{}
	err := sagarm.HandleCommit(ctx, rm.db, gtid, bid, func(tx *gorm.DB) error {
		txr := tx.Model(BankAccount{}).Where("id=?", in.GetUserId()).
			Update("balance", gorm.Expr("balance+?", in.GetAccount()))
		if txr.Error != nil {
			return status.Error(codes.Internal, txr.Error.Error())
		}
		if txr.RowsAffected == 0 {
			// logutil.Logger(ctx).Sugar().Debugf("grpc In : %v, %v", in.GetUserId(), in.GetAccount())
			return status.Error(codes.NotFound, "")
		}
		return nil
	})
	//logutil.Logger(ctx).Sugar().Debugf("grpc In : %s ,  %v, %v, %v", gtid, bid, in, err)
	return resp, err
}

func (rm *SagaRmBankGrpcService) Out(ctx context.Context, in *pb.SagaRequest) (*pb.SagaResponse, error) {
	gtid, bid := cgrpc.ParseContextMeta(ctx)
	resp := &pb.SagaResponse{}

	err := sagarm.HandleCompensation(ctx, rm.db, gtid, bid, func(tx *gorm.DB) error {
		txr := tx.Model(BankAccount{}).Where("id=?", in.GetUserId()).
			Update("balance", gorm.Expr("balance+?", -1*in.GetAccount()))
		if txr.Error != nil {
			return status.Error(codes.Internal, txr.Error.Error())
		}
		if txr.RowsAffected == 0 {
			// logutil.Logger(ctx).Sugar().Debugf("grpc Out : %v, %v", in.GetUserId(), in.GetAccount())
			return status.Error(codes.NotFound, "")
		}
		return nil
	})
	//logutil.Logger(ctx).Sugar().Debugf("grpc Out : %s ,  %v, %v, %v", gtid, bid, in, err)
	return resp, err
}

func (rm *SagaRmBankGrpcService) Start() error {
	logutil.Logger(context.TODO()).Sugar().Debugf("saga rm grpc server : %v", rm.listen)

	lis, err := net.Listen("tcp", rm.listen)
	if err != nil {
		return err
	}

	server := grpc.NewServer()
	pb.RegisterSagaBankServiceServer(server, rm)
	reflection.Register(server)
	return server.Serve(lis)
}
