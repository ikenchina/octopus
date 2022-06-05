package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	saga_cli "github.com/ikenchina/octopus/client/saga"
	logutil "github.com/ikenchina/octopus/common/log"
	"github.com/ikenchina/octopus/define"
	saga_rm "github.com/ikenchina/octopus/test/utils/saga"
)

func (app *Application) PayWage(employees []*saga_rm.BankAccountRecord) (*define.SagaResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	notifyAction := fmt.Sprintf("http://localhost%s/saga/notify", app.listen)
	transactionExpiredTime := time.Now().Add(1 * time.Minute)

	resp, err := saga_cli.SagaTransaction(ctx, tcDomain, transactionExpiredTime,
		func(t *saga_cli.Transaction, gtid string) error {

			app.saveGtidToDb(gtid)

			// set notify
			t.SetNotify(notifyAction, time.Second, time.Second)

			for i, employee := range employees {
				branchID := i + 1
				actionURL := fmt.Sprintf("%s%s/%s/%d", app.employeeHosts[employee.UserID], saga_rm.SagaRmBankServiceBasePath, gtid, branchID)
				t.NewBranch(branchID, actionURL, actionURL, jsonMarshal(employee))
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	app.updateTccStateToDb(resp.Gtid, resp.State)
	return resp, err
}

func (app *Application) notifyHandler(c *gin.Context) {
	// update database : distributed transaction is commited or aborted

	body, err := c.GetRawData()
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	logutil.Logger(context.TODO()).Sugar().Debugf("notify : %s", string(body))
}

var (
	tcListen = ":8080"
	tcDomain = fmt.Sprintf("http://localhost%s", tcListen)
)

type Application struct {
	employeeHosts map[int]string
	listen        string
	Db            *gorm.DB
	httpServer    *http.Server
}

func (app *Application) start() error {
	ginApp := gin.New()
	ginApp.POST("/saga/notify", app.notifyHandler)
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

func (app *Application) updateTccStateToDb(gtid string, state string) {
}

func jsonMarshal(user *saga_rm.BankAccountRecord) string {
	b, _ := json.Marshal(user)
	return string(b)
}
