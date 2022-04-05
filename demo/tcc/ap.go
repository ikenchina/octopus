package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	tcc_cli "github.com/ikenchina/octopus/client/tcc"
	"github.com/ikenchina/octopus/define"
)

var (
	tcListen = ":8080"
	tcDomain = fmt.Sprintf("http://localhost%s", tcListen)
)

type Application struct {
	rmAHost string
	rmBHost string
}

func (app *Application) Transfer(userID, otherUserID int, account int) (*define.TccResponse, error) {
	tccCtx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	transactionExpireTime := time.Now().Add(time.Second)

	tccResp, err := tcc_cli.TccTransaction(tccCtx, tcDomain, transactionExpireTime,
		func(t *tcc_cli.Transaction, gtid string) error {

			// save gtid to database : recovery from db after crash
			app.saveGtidToDb(gtid)

			// try service a
			branchId := 1
			actionAURL := fmt.Sprintf("%s%s/%s/%d", app.rmAHost, service_BasePath, gtid, branchId)
			tryAresp, err := t.Try(branchId, actionAURL, actionAURL, actionAURL, jsonMarshal(&AccountRecord{
				UserID:  userID,
				Account: -1 * account,
			}))
			if err != nil {
				return err
			}

			// try service b
			branchId++
			actionBURL := fmt.Sprintf("%s%s/%s/%d", app.rmBHost, service_BasePath, gtid, branchId)
			tryBresp, err := t.Try(branchId, actionBURL, actionBURL, actionBURL, jsonMarshal(&AccountRecord{
				UserID:  otherUserID,
				Account: account,
			}))
			if err != nil {
				return err
			}
			app.saveResponse(tryAresp)
			app.saveResponse(tryBresp)

			return nil
		})

	app.updateTccStateToDb(tccResp.Gtid, tccResp.State)
	return tccResp, err
}

func (app *Application) saveResponse(r []byte) {
	//logutil.Logger(context.TODO()).Sugar().Debugf("saveResponse : %s", string(r))
}

func (app *Application) saveGtidToDb(gtid string) {
	// save gtid to database, query tc when recovering
	// or wait for transaction to expire
}

func (app *Application) updateTccStateToDb(gtid string, state string) {
	//logutil.Logger(context.TODO()).Sugar().Debugf("updateTccStateToDb : %s %s", gtid, state)
}

func jsonMarshal(user *AccountRecord) string {
	b, _ := json.Marshal(user)
	return string(b)
}
