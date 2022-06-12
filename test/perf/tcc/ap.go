package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/ikenchina/octopus/define"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	tcc_cli "github.com/ikenchina/octopus/client/tcc"
	tcc_rm "github.com/ikenchina/octopus/test/utils/tcc"
)

func (app *Application) Transfer(userA, userB int, account int) (*define.TccResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	transactionExpiredTime := time.Now().Add(1 * time.Minute)
	resp, err := tcc_cli.TccTransaction(ctx, config.Ap.TcDomain, transactionExpiredTime,
		func(t *tcc_cli.Transaction, gtid string) error {

			app.saveGtidToDb(gtid)

			// 尝试调用账户A的所属银行服务器的try接口
			branchId := 1
			actionAURL := fmt.Sprintf("%s%s/%s/%d", app.employeeHosts[0], tcc_rm.TccRmBankServiceBasePath, gtid, branchId)

			// 调用tcc_cli.Transaction的Try方法，会将此子事务注册到TC，再调用RM的try接口
			// tryAresp为RM的try接口返回的响应body
			tryAresp, err := t.Try(branchId, actionAURL, actionAURL, actionAURL, jsonMarshal(&tcc_rm.BankAccountRecord{
				UserID:  userA,
				Account: -1 * account,
			}))
			if err != nil {
				return err
			}

			// 尝试调用账户B所属银行服务器的try接口
			branchId++
			actionBURL := fmt.Sprintf("%s%s/%s/%d", app.employeeHosts[1], tcc_rm.TccRmBankServiceBasePath, gtid, branchId)
			tryBresp, err := t.Try(branchId, actionBURL, actionBURL, actionBURL, jsonMarshal(&tcc_rm.BankAccountRecord{
				UserID:  userB,
				Account: account,
			}))
			if err != nil {
				return err
			}
			app.saveResponse(tryAresp)
			app.saveResponse(tryBresp)

			return nil
		})

	if err != nil {
		return nil, err
	}
	app.updateTccStateToDb(resp.Gtid, resp.State)
	return resp, err
}

type Application struct {
	employeeHosts []string
	listen        string
	httpServer    *http.Server
}

func (app *Application) Start() error {
	ginApp := gin.New()
	ginApp.GET("/debug/metrics", gin.WrapH(promhttp.Handler()))
	pprof.Register(ginApp, "debug/pprof")
	app.httpServer = &http.Server{
		Addr:    app.listen,
		Handler: ginApp,
	}
	return app.httpServer.ListenAndServe()
}

func (app *Application) saveGtidToDb(gtid string) {
	// save gtid to database, query tc when recovering
	// or waitting to be notifed by tc
}

func (app *Application) saveResponse(payload []byte) {

}

func (app *Application) updateTccStateToDb(gtid string, state string) {
}

func jsonMarshal(user *tcc_rm.BankAccountRecord) string {
	b, _ := json.Marshal(user)
	return string(b)
}
