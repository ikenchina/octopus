package ap

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	tcc_cli "github.com/ikenchina/octopus/ap/tcc"
	"github.com/ikenchina/octopus/define/proto/tcc/pb"
	tcc_rm "github.com/ikenchina/octopus/test/rm/tcc"
	tcc_pb "github.com/ikenchina/octopus/test/rm/tcc/pb"
)

func (app *TccApplication) Transfer(users []*tcc_rm.BankAccountRecord) (*pb.TccResponse, error) {
	tccCtx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	transactionExpireTime := time.Now().Add(time.Second)

	tccResp, err := tcc_cli.TccTransaction(tccCtx, app.tcClient, transactionExpireTime,
		func(ctx context.Context, t *tcc_cli.Transaction, gtid string) error {

			// save gtid to database : recovery from db after crash
			app.saveGtidToDb(gtid)

			for i, user := range users {
				branchID := i + 1
				host := app.banks[int(math.Abs(float64(user.UserID)))%len(app.banks)]

				// 如果银行是gRPC接口
				if host.Protocol == "grpc" {
					cli := app.getBankGrpcClient(host.Target)
					response := &pb.TccResponse{}
					request := &tcc_pb.TccRequest{
						UserId:  int32(user.UserID),
						Account: int32(user.Account),
					}

					// 调用tc的Try接口，注册这个分支事务
					err := t.TryGrpc(
						branchID, // 分支事务ID
						cli,      // rm的grpc connection
						"/bankservice.TccBankService/Try",
						"/bankservice.TccBankService/Confirm", // 银行的Confirm接口，由TC调用
						"/bankservice.TccBankService/Cancel",  // 银行的Cancel接口，由TC调用
						request,                               // grpc try请求结构
						response)                              // grpc try请求返回结构
					if err != nil {
						return err
					}
				} else if host.Protocol == "http" { // 如果银行是http接口
					// try, confirm, cancel为同一个URL，由http method区分，分别为POST, PUT, DELETE
					actionAURL := fmt.Sprintf("%s%s/%s/%d", host.Target, tcc_rm.TccRmBankServiceBasePath, gtid, branchID)

					// Transaction提供了TryHttp接口，实现调用try的逻辑；也可以自行调用对方try接口
					_, err := t.TryHttp(branchID,
						actionAURL, // try接口
						actionAURL, // confirm接口
						actionAURL, // cancel接口
						jsonMarshal(&tcc_rm.BankAccountRecord{
							UserID:  user.UserID,
							Account: user.Account,
						}))
					if err != nil {
						return err
					}
				}
			}

			return nil
		})

	app.updateStateToDb(tccResp.GetTcc().GetGtid(), tccResp.GetTcc().GetState())
	return tccResp, err
}

func (app *TccApplication) InitTcClient(tcDomain string) (err error) {
	app.tcClient, err = tcc_cli.NewGrpcClient(tcDomain)
	return err
}

type TccApplication struct {
	Application
	clis     map[string]*grpc.ClientConn
	tcClient *tcc_cli.GrpcClient
}

func (app *TccApplication) getBankGrpcClient(target string) *grpc.ClientConn {
	app.mutex.RLock()
	if app.clis == nil {
		app.clis = make(map[string]*grpc.ClientConn)
	}
	c, ok := app.clis[target]
	if ok {
		app.mutex.RUnlock()
		return c
	}
	app.mutex.RUnlock()

	conn, err := grpc.Dial(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Panic(err)
		return nil
	}

	app.mutex.Lock()
	app.clis[target] = conn
	app.mutex.Unlock()

	return conn
}
