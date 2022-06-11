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

	"go.uber.org/ratelimit"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/ikenchina/octopus/common/errorutil"
	logutil "github.com/ikenchina/octopus/common/log"
	"github.com/ikenchina/octopus/common/metrics"
	"github.com/ikenchina/octopus/define"
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

	gin.SetMode(gin.DebugMode)
	logutil.Logger(context.Background()).Debug("Demonstration")
	rmcommon.SetDbSchema("dtx")

	data, err := ioutil.ReadFile(*cfgFlag)
	errorutil.PanicIfError(err)
	errorutil.PanicIfError(json.Unmarshal(data, &config))

	if config.Log != nil {
		logutil.InitLog(config.Log)
	}

	switch *cmdFlag {
	case "ap":
		ap()
	case "rm":
		startRm()
	}
}

func startRm() {
	fmt.Printf("rm config : %+v\n", config.Ap)

	wait := sync.WaitGroup{}
	for _, rm := range config.Rm {
		db, err := gorm.Open(postgres.Open(rm.Dsn), &gorm.Config{
			SkipDefaultTransaction: true,
		})
		errorutil.PanicIfError(err)

		sdb, err := db.DB()
		errorutil.PanicIfError(err)
		sdb.SetMaxOpenConns(rm.MaxConnections)
		sdb.SetMaxIdleConns(rm.MaxIdleConnections)

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
	logutil.Logger(context.TODO()).Sugar().Debugf("ap config : %+v\n", config.Ap)
	app := startAp()
	chSize := config.Ap.Qps * 2
	if chSize < 0 {
		chSize = 1
	}
	ch := make(chan struct{}, chSize)
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

	consume := func() {
		timer := apTimer.Timer()
		//wage := rand.Intn(100)
		rr, _ := constructRecords2(10)
		resp, err := app.PayWage(rr)
		if err != nil {
			logutil.Logger(context.Background()).Sugar().Debugf("err : %v", err)
			timer("", "err")
		} else {
			timer(resp.State, "ok")
			if resp.State != define.TxnStateCommitted {
				logutil.Logger(context.Background()).Sugar().Debugf("abort : %s=%s %v", resp.Gtid, resp.State, err)
			}
		}
	}

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

var (
	user = 0
	lock sync.Mutex
)

func constructRecords2(wage int) ([]*saga_rm.BankAccountRecord, bool) {
	records := []*saga_rm.BankAccountRecord{}
	lock.Lock()
	user++
	u2 := user
	lock.Unlock()

	records = append(records, &saga_rm.BankAccountRecord{
		UserID:  u2 * -1,
		Account: config.Ap.SagaSize * wage * -1,
	})
	for i := 0; i < config.Ap.SagaSize; i++ {
		records = append(records, &saga_rm.BankAccountRecord{
			UserID:  u2,
			Account: wage,
		})
		//user++
	}
	return records, false
}

func startAp() *Application {
	//"postgresql://dtx_user:dtx_pass@127.0.0.1:5432/dtx?connect_timeout=3"
	app := &Application{
		listen: config.Ap.Listen,
	}

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
