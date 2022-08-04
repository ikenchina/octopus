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

	"github.com/gin-gonic/gin"
	"go.uber.org/ratelimit"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/ikenchina/octopus/common/errorutil"
	logutil "github.com/ikenchina/octopus/common/log"
	"github.com/ikenchina/octopus/common/metrics"
	"github.com/ikenchina/octopus/define"
	rmcommon "github.com/ikenchina/octopus/rm/common"
	ap "github.com/ikenchina/octopus/test/ap"
	saga_rm "github.com/ikenchina/octopus/test/rm/saga"
	tcc_rm "github.com/ikenchina/octopus/test/rm/tcc"
)

var (
	roleFlag = flag.String("role", "ap", "application")
	txnFlag  = flag.String("txn", "saga", "txn type")
	cfgFlag  = flag.String("config", "./config.json", "config path")
	dryRun   = flag.Bool("dry", false, "dry run")

	config Config
)

func main() {
	flag.Parse()
	cfg := zap.NewDevelopmentConfig()
	logutil.InitLog(&cfg)
	gin.SetMode(gin.DebugMode)
	rmcommon.SetDbSchema("dtx")

	data, err := ioutil.ReadFile(*cfgFlag)
	errorutil.PanicIfError(err)
	errorutil.PanicIfError(json.Unmarshal(data, &config))
	if config.Log != nil {
		logutil.InitLog(config.Log)
	}

	switch *roleFlag {
	case "ap":
		runAp()
	case "rm":
		startRm()
	}
}

func runAp() {
	logutil.Logger(context.TODO()).Sugar().Debugf("ap config : %+v\n", config.Ap)
	if *txnFlag == "saga" {
		app := startSagaAp()
		consume := func() {
			timer := apTimer.Timer()
			//wage := rand.Intn(100)
			rr, _ := sagaRecords2(10)
			resp, err := app.Pay(rr)
			if err != nil {
				logutil.Logger(context.Background()).Sugar().Debugf("err : %v", err)
				timer("", "err")
			} else {
				timer(resp.Saga.GetState(), "ok")
				if resp.Saga.GetState() != define.TxnStateCommitted {
					logutil.Logger(context.Background()).Sugar().Debugf("abort : %s=%s %v", resp.Saga.GetGtid(), resp.GetSaga().GetState(), err)
				}
			}
		}
		limitRun(consume)
	} else {
		app := startTccAp()
		consume := func() {
			timer := apTimer.Timer()
			//wage := rand.Intn(100)
			rr, _ := tccRecords2(10)
			resp, err := app.Transfer(rr)
			if err != nil {
				logutil.Logger(context.Background()).Sugar().Debugf("err : %v", err)
				timer("", "err")
			} else {
				timer(resp.GetTcc().GetState(), "ok")
				if resp.GetTcc().GetState() != define.TxnStateCommitted {
					logutil.Logger(context.Background()).Sugar().Debugf("abort : %s=%s %v", resp.Tcc.GetGtid(), resp.GetTcc().GetState(), err)
				}
			}
		}
		limitRun(consume)
	}

}

func limitRun(consume func()) {
	chSize := config.Ap.Qps * 2
	if chSize < 0 {
		chSize = 1
	}
	ch := make(chan struct{}, chSize)

	if *dryRun {
		<-ch
		return
	}

	go func() {
		if config.Ap.Qps < 0 {
			return
		}
		limiter := ratelimit.New(config.Ap.Qps)
		for {
			limiter.Take()
			ch <- struct{}{}
		}
	}()

	wait := sync.WaitGroup{}
	for i := 0; i < config.Ap.Concurrency; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range ch {
				consume()
			}
		}()
	}
	wait.Wait()
}

func sagaRecords2(wage int) ([]*saga_rm.BankAccountRecord, bool) {
	records := []*saga_rm.BankAccountRecord{}
	userCount := config.Ap.UserRange[1] - config.Ap.UserRange[0] + 1
	user1 := rand.Intn(userCount) + config.Ap.UserRange[0]
	bizUser := user1 * -1
	fail := rand.Float32() < config.Ap.FailRate
	records = append(records, &saga_rm.BankAccountRecord{
		UserID:  bizUser,
		Account: config.Ap.TxnSize * wage * -1,
	})
	failIdx := rand.Intn(config.Ap.TxnSize)
	for i := 0; i < config.Ap.TxnSize; i++ {
		user := (user1+i)%userCount + config.Ap.UserRange[0]
		records = append(records, &saga_rm.BankAccountRecord{
			UserID:  user,
			Account: wage,
			Fail:    (i == failIdx && fail),
		})
	}
	return records, fail
}

func tccRecords2(wage int) ([]*tcc_rm.BankAccountRecord, bool) {
	records := []*tcc_rm.BankAccountRecord{}
	userCount := config.Ap.UserRange[1] - config.Ap.UserRange[0] + 1 - config.Ap.TxnSize
	user1 := rand.Intn(userCount) + config.Ap.UserRange[0]
	bizUser := user1 * -1
	fail := rand.Float32() < config.Ap.FailRate
	records = append(records, &tcc_rm.BankAccountRecord{
		UserID:  bizUser,
		Account: config.Ap.TxnSize * wage * -1,
	})
	failIdx := rand.Intn(config.Ap.TxnSize)
	for i := 0; i < config.Ap.TxnSize; i++ {
		user := user1 + i
		records = append(records, &tcc_rm.BankAccountRecord{
			UserID:  user,
			Account: wage,
			Fail:    (i == failIdx && fail),
		})
	}
	return records, fail
}

var (
	user = 0
	lock sync.Mutex
)

func sagaRecords(wage int) ([]*saga_rm.BankAccountRecord, bool) {
	records := []*saga_rm.BankAccountRecord{}
	lock.Lock()
	user++
	u2 := user
	lock.Unlock()

	records = append(records, &saga_rm.BankAccountRecord{
		UserID:  u2 * -1,
		Account: config.Ap.TxnSize * wage * -1,
	})
	for i := 0; i < config.Ap.TxnSize; i++ {
		records = append(records, &saga_rm.BankAccountRecord{
			UserID:  u2,
			Account: wage,
		})
		//user++
	}
	return records, false
}

func startSagaAp() *ap.SagaApplication {
	app := &ap.SagaApplication{}
	app.InitTcClient(config.Ap.TcDomain)
	initAp(&app.Application)
	return app
}

func startTccAp() *ap.TccApplication {
	app := &ap.TccApplication{}
	app.InitTcClient(config.Ap.TcDomain)
	initAp(&app.Application)
	return app
}

func initAp(app *ap.Application) {
	go func() {
		errorutil.PanicIfError(app.InitHttp(config.Ap.Listen))
	}()

	app.SetNotifyUrl(fmt.Sprintf("%s%s/saga/notify", config.Ap.Notify, config.Ap.Listen))
	for _, rm := range config.Rm {
		host, port := extractListen(rm.Listen)
		r := fmt.Sprintf("%s:%s", host, port)
		if rm.Protocol == "http" {
			r = "http://" + r
		}
		app.AddBank(ap.Bank{
			Protocol: rm.Protocol,
			Port:     port,
			Target:   r,
		})
	}
}

func startRm() {
	fmt.Printf("rm config : %+v\n", config.Ap)

	wait := sync.WaitGroup{}
	for _, rmt := range config.Rm {
		db, err := gorm.Open(postgres.Open(rmt.Dsn), &gorm.Config{
			SkipDefaultTransaction: true,
		})
		errorutil.PanicIfError(err)

		sdb, err := db.DB()
		errorutil.PanicIfError(err)
		sdb.SetMaxOpenConns(rmt.MaxConnections)
		sdb.SetMaxIdleConns(rmt.MaxIdleConnections)
		rm := rmt
		if *txnFlag == "saga" {
			wait.Add(1)
			go func() {
				defer wait.Done()
				errorutil.PanicIfError(saga_rm.StartRm(rm.Listen, db, rm.Protocol))
			}()
		} else if *txnFlag == "tcc" {
			wait.Add(1)
			go func() {
				defer wait.Done()
				errorutil.PanicIfError(tcc_rm.StartRm(rm.Listen, db, rm.Protocol))
			}()
		}
	}
	wait.Wait()
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

var (
	apTimer = metrics.NewTimer("dtx", "ap", "consume", "ap timer", []string{"state", "ret"})
)
