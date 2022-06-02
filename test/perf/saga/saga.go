package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"strings"
	"sync"

	"golang.org/x/time/rate"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/ikenchina/octopus/common/errorutil"
	logutil "github.com/ikenchina/octopus/common/log"
	"github.com/ikenchina/octopus/common/metrics"
	rmcommon "github.com/ikenchina/octopus/rm/common"
	saga_rm "github.com/ikenchina/octopus/test/utils/saga"
)

var (
	cmdFlag = flag.String("cmd", "ap", "application")
	cfgFlag = flag.String("config", "./config.json", "config path")

	config Config
)

func main() {
	flag.Parse()

	logger, _ := zap.NewDevelopment()
	logutil.SetLogger(logger)
	gin.SetMode(gin.DebugMode)
	logutil.Logger(context.Background()).Debug("Demonstration")
	rmcommon.SetDbSchema("dtx")

	data, err := ioutil.ReadFile(*cfgFlag)
	errorutil.PanicIfError(err)
	errorutil.PanicIfError(json.Unmarshal(data, &config))

	switch *cmdFlag {
	case "ap":
		ap()
	case "rm":
		startRm()
	}
}

func startRm() {
	wait := sync.WaitGroup{}
	for _, rm := range config.Rm {
		db, err := gorm.Open(postgres.Open(rm.Dsn), &gorm.Config{
			SkipDefaultTransaction: true,
		})
		errorutil.PanicIfError(err)
		srm := saga_rm.NewSagaRmBankService(rm.Listen, db)
		wait.Add(1)
		go func() {
			defer wait.Done()
			errorutil.PanicIfError(srm.Start())
		}()
	}
	wait.Wait()
}

var (
	apTimer = metrics.NewTimer("dtx", "ap", "ap timer", []string{"state", "ret"})
)

func ap() {

	app := startAp()
	ch := make(chan struct{}, config.Ap.Qps*2)
	go func() {
		limiter := rate.NewLimiter(rate.Limit(config.Ap.Qps), config.Ap.Qps*2)
		for {
			limiter.Wait(context.Background())
			ch <- struct{}{}
		}
	}()

	consume := func() {
		timer := apTimer.Timer()
		wage := rand.Intn(100)
		rr, _ := constructRecords(wage)
		resp, err := app.PayWage(rr)
		if err != nil {
			timer("", "err")
		} else {
			timer(resp.State, "ok")
		}
	}

	wait := sync.WaitGroup{}
	for i := 0; i < config.Ap.Concurrency; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for {
				consume()
			}
		}()
	}
	wait.Wait()
}

func constructRecords(wage int) ([]*saga_rm.BankAccountRecord, bool) {
	records := []*saga_rm.BankAccountRecord{}
	userCount := config.Ap.UserRange[1] - config.Ap.UserRange[0] + 1
	user1 := rand.Intn(userCount) + config.Ap.UserRange[0]
	bizUser := user1 * -1
	fail := rand.Float32() < config.Ap.FailRate
	records = append(records, &saga_rm.BankAccountRecord{
		UserID:  bizUser,
		Account: config.Ap.SagaSize * wage * -1,
	})
	failIdx := rand.Intn(config.Ap.SagaSize)
	for i := 0; i < config.Ap.SagaSize; i++ {
		user := (user1+i)%config.Ap.UserRange[1] + config.Ap.UserRange[0]
		records = append(records, &saga_rm.BankAccountRecord{
			UserID:  user,
			Account: wage,
			Fail:    (i == failIdx && fail),
		})
	}
	return records, fail
}

func startAp() *Application {
	//"postgresql://dtx_user:dtx_pass@127.0.0.1:5432/dtx?connect_timeout=3"
	app := &Application{
		listen: config.Ap.Listen,
	}

	db, err := gorm.Open(postgres.Open(config.Ap.Dsn), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	errorutil.PanicIfError(err)
	app.Db = db
	sdb, _ := db.DB()
	sdb.SetMaxOpenConns(10)
	sdb.SetMaxIdleConns(5)

	go func() {
		errorutil.PanicIfError(app.Start())
	}()

	for _, rm := range config.Rm {
		host, port := extractListen(rm.Listen)
		r := fmt.Sprintf("http://%s:%s", host, port)
		app.employeeHosts = append(app.employeeHosts, r)
	}

	return app
}

func extractListen(listen string) (string, string) {
	n := strings.Split(listen, ":")
	if len(n) != 2 {
		logutil.Logger(context.Background()).Sugar().Panic("listen config error ", listen)
	}
	if n[0] == "" {
		return "localhost", n[1]
	}
	return n[0], n[1]
}
