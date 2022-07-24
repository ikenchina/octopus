package tcc

import (
	"context"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	sgrpc "github.com/ikenchina/octopus/common/grpc"
	logutil "github.com/ikenchina/octopus/common/log"
	tccrm "github.com/ikenchina/octopus/rm/tcc"
	"github.com/ikenchina/octopus/test/rm/tcc/pb"
)

type TccRmBankGrpcService struct {
	pb.UnimplementedTccBankServiceServer
	grpcServer *grpc.Server
	listen     string
	db         *gorm.DB
}

func NewTccRmBankGrpcService(listen string, db *gorm.DB) *TccRmBankGrpcService {
	return &TccRmBankGrpcService{
		listen: listen,
		db:     db,
	}
}

func (rm *TccRmBankGrpcService) Try(ctx context.Context, request *pb.TccRequest) (*pb.TccResponse, error) {
	defer rmExecTimer.Timer()("try")

	gtid, branchID := sgrpc.ParseContextMeta(ctx)

	err := tccrm.HandleTryOrm(ctx, rm.db, gtid, branchID,
		func(tx *gorm.DB) error {
			txr := tx.Model(BankAccount{}).
				//Where("id=? AND balance_freeze=0 AND balance+?>=0",
				Where("id=? AND balance_freeze=0 ", request.GetUserId()).
				Update("balance_freeze", request.GetAccount())

			if txr.Error != nil {
				return status.Error(codes.Internal, txr.Error.Error())
			}
			if txr.RowsAffected == 0 {
				return status.Error(codes.FailedPrecondition, "insufficient balance or freeze != 0")
			}
			return nil
		})
	if err != nil {
		logutil.Logger(context.TODO()).Sugar().Errorf("try err : %s %d %v", gtid, branchID, err)
		return nil, err
	}

	logutil.Logger(ctx).Sugar().Debugf("grpc try : %s, %v, %v", gtid, branchID, err)

	return &pb.TccResponse{}, nil
}

func (rm *TccRmBankGrpcService) Confirm(ctx context.Context, request *pb.TccRequest) (*pb.TccResponse, error) {
	defer rmExecTimer.Timer()("confirm")

	gtid, branchID := sgrpc.ParseContextMeta(ctx)

	err := tccrm.HandleConfirmOrm(ctx, rm.db, gtid, branchID,
		func(tx *gorm.DB) error {
			// 将用户冻结资金加到账户余额中，同时清空冻结资金列
			//
			txr := tx.Model(BankAccount{}).Where("id=?", request.GetUserId()).
				Updates(map[string]interface{}{
					"balance":        gorm.Expr("balance+balance_freeze"),
					"balance_freeze": 0})
			if txr.Error != nil {
				return status.Error(codes.Internal, txr.Error.Error())
			}
			if txr.RowsAffected == 0 {
				return status.Error(codes.FailedPrecondition, "insufficient balance or freeze != 0")
			}
			return nil
		})

	if err != nil {
		logutil.Logger(context.TODO()).Sugar().Errorf("confirm err : %s %d %v", gtid, branchID, err)
		return nil, err
	}

	logutil.Logger(ctx).Sugar().Debugf("grpc confirm : %s, %v, %v", gtid, branchID, err)

	return &pb.TccResponse{}, nil
}

func (rm *TccRmBankGrpcService) Cancel(ctx context.Context, request *pb.TccRequest) (*pb.TccResponse, error) {
	defer rmExecTimer.Timer()("cancel")

	gtid, branchID := sgrpc.ParseContextMeta(ctx)

	err := tccrm.HandleCancelOrm(ctx, rm.db, gtid, branchID,
		func(tx *gorm.DB) error {
			// 取消事务，将冻结资金列清空
			txr := tx.Model(BankAccount{}).Where("id=?", request.GetUserId()).Update(
				"balance_freeze", 0)
			if txr.Error != nil {
				return status.Error(codes.Internal, txr.Error.Error())
			}
			if txr.RowsAffected == 0 {
				return status.Error(codes.FailedPrecondition, "insufficient balance or freeze != 0")
			}
			return nil
		})

	if err != nil {
		logutil.Logger(context.TODO()).Sugar().Errorf("cancel err : %s %d %v", gtid, branchID, err)
		return nil, err
	}
	logutil.Logger(ctx).Sugar().Debugf("grpc cancel : %s, %v, %v", gtid, branchID, err)

	return &pb.TccResponse{}, nil
}

func (rm *TccRmBankGrpcService) Start() error {
	logutil.Logger(context.Background()).Sugar().Debugf("tcc grpc start : %s", rm.listen)

	lis, err := net.Listen("tcp", rm.listen)
	if err != nil {
		return err
	}

	server := grpc.NewServer()
	pb.RegisterTccBankServiceServer(server, rm)
	reflection.Register(server)
	return server.Serve(lis)
}
