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

func main() {
	initLog()
	rmcommon.SetDbSchema("dtx")

	rmAPort := ":8087"
	rmAHost := fmt.Sprintf("http://localhost%s", rmAPort)
	rmBPort := ":8088"
	rmBHost := fmt.Sprintf("http://localhost%s", rmBPort)

	db, err := gorm.Open(postgres.Open("postgresql://dtx_user:dtx_pass@127.0.0.1:5432/dtx?connect_timeout=3"), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	errorutil.PanicIfError(err)

	rma := &RmServiceA{}
	rmb := &RmServiceB{}
	go func() {
		errorutil.PanicIfError(rma.start(rmAPort, db))
	}()
	go func() {
		errorutil.PanicIfError(rmb.start(rmBPort, db))
	}()

	app := &Application{
		rmAHost: rmAHost,
		rmBHost: rmBHost,
	}

	resp, err := app.Transfer(1, 2, 10)
	logutil.Logger(context.Background()).Sugar().Debugf("transfer : %v %v ", err, resp)
}

func initLog() {
	logger, _ := zap.NewDevelopment()
	logutil.SetLogger(logger)
	gin.SetMode(gin.DebugMode)
	logutil.Logger(context.Background()).Debug("Demonstration")
}
