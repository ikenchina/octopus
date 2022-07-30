package ap

import (
	"context"
	"fmt"
	"math"
	"time"

	saga_cli "github.com/ikenchina/octopus/ap/saga"
	"github.com/ikenchina/octopus/define/proto/saga/pb"
	saga_rm "github.com/ikenchina/octopus/test/rm/saga"
)

func (app *SagaApplication) Pay(users []*saga_rm.BankAccountRecord) (*pb.SagaResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	notifyAction := app.notifyUrl
	transactionExpiredTime := time.Now().Add(1 * time.Minute)

	resp, err := saga_cli.SagaTransaction(ctx, app.tcClient, transactionExpiredTime,
		func(t *saga_cli.Transaction, gtid string) error {

			app.saveGtidToDb(gtid)

			// set notify
			t.SetHttpNotify(notifyAction, time.Second, time.Second)

			for i, user := range users {
				branchID := i + 1
				host := app.getUserBank(user.UserID)
				if host.Protocol == "grpc" {
					commit := "/bankservice.SagaBankService/In"
					compensation := "/bankservice.SagaBankService/Out"
					payload := user.ToPb()
					t.AddGrpcBranch(branchID, host.Target, commit, compensation, payload, saga_cli.WithMaxRetry(1))
				} else if host.Protocol == "http" {
					actionURL := fmt.Sprintf("%s%s/%s/%d", host.Target, saga_rm.SagaRmBankServiceBasePath, gtid, branchID)
					payload := jsonMarshal(user)
					t.AddHttpBranch(branchID, actionURL, actionURL, payload, saga_cli.WithMaxRetry(1))
				}
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	app.updateStateToDb(resp.Saga.GetGtid(), resp.Saga.GetState())
	return resp, err
}

func (app *SagaApplication) getUserBank(userID int) Bank {
	return app.banks[int(math.Abs(float64(userID)))%len(app.banks)]
}

func (app *SagaApplication) InitTcClient(tcDomain string) (err error) {
	app.tcClient, err = saga_cli.NewGrpcClient(tcDomain)
	return err
}

type SagaApplication struct {
	Application
	tcClient *saga_cli.GrpcClient
}
