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
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/ikenchina/octopus/common/errorutil"
	logutil "github.com/ikenchina/octopus/common/log"
	"github.com/ikenchina/octopus/common/metrics"
	"github.com/ikenchina/octopus/define"
	rmcommon "github.com/ikenchina/octopus/rm/common"
	tcc_rm "github.com/ikenchina/octopus/test/utils/tcc"
)

var (
	cmdFlag = flag.String("cmd", "ap", "application")
	cfgFlag = flag.String("config", "./config.json", "config path")
	config  Config
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
	logutil.Logger(context.Background()).Sugar().Infof("rm config : %+v\n", config.Ap)

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

		srm := tcc_rm.NewTccRmBankService(rm.Listen, db)
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
	logutil.Logger(context.TODO()).Sugar().Infof("ap config : %+v\n", config.Ap)
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
		a, b, wage := constructRecordsRandom(10)
		resp, err := app.Transfer(a, b, wage)
		if err != nil {
			logutil.Logger(context.Background()).Sugar().Errorf("err : %v", err)
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

var (
	user = 0
	lock sync.Mutex
)

func constructRecords2(wage int) (int, int, int) {
	lock.Lock()
	defer lock.Unlock()
	user++
	return user * -1, user, wage
}

func constructRecordsRandom(wage int) (int, int, int) {
	user := config.Ap.UserRange[0] + rand.Intn(config.Ap.UserRange[1]-config.Ap.UserRange[0])
	return user * -1, user, wage
}
