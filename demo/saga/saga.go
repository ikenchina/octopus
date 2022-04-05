package main

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/ikenchina/octopus/common/errorutil"
	logutil "github.com/ikenchina/octopus/common/log"
	rmcommon "github.com/ikenchina/octopus/rm/common"
)

var (
	portStart = 10001
	app       = &Application{
		employeeHosts: make(map[int]string),
		listen:        fmt.Sprintf(":%d", portStart-1),
	}
)

func main() {
	initAp()

	resp, err := app.PayWage(constructRecords())
	logutil.Logger(context.Background()).Sugar().Debugf("pay wages : %v %v", resp, err)
}

func constructRecords() []*AccountRecord {
	records := []*AccountRecord{}
	employeeCount := 5
	wage := 10

	for i := 0; i < employeeCount; i++ {
		port := portStart + i
		host := fmt.Sprintf("http://localhost:%d", port)
		app.employeeHosts[i] = host
		rm := &RmService{
			listen: fmt.Sprintf(":%v", port),
			Db:     app.Db,
		}
		go func() {
			errorutil.PanicIfError(rm.start())
		}()
		if i == 0 {
			records = append(records, &AccountRecord{
				UserID:  i, // account of company
				Account: -1 * (employeeCount - 1) * wage,
			})
		} else { // employees' account
			records = append(records, &AccountRecord{
				UserID:  i,
				Account: wage,
			})
		}
	}
	return records
}

func initAp() {
	logger, _ := zap.NewDevelopment()
	logutil.SetLogger(logger)
	gin.SetMode(gin.DebugMode)
	logutil.Logger(context.Background()).Debug("Demonstration")
	rmcommon.SetDbSchema("dtx")

	db, err := gorm.Open(postgres.Open("postgresql://dtx_user:dtx_pass@127.0.0.1:5432/dtx?connect_timeout=3"), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	errorutil.PanicIfError(err)

	app.Db = db
	go func() {
		errorutil.PanicIfError(app.start())
	}()

}
