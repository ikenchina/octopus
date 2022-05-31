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
	"time"

	"golang.org/x/time/rate"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/ikenchina/octopus/common/errorutil"
	logutil "github.com/ikenchina/octopus/common/log"
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

type stats struct {
	Start      time.Time
	Err        int64
	ErrLatency int64
	Total      int64
	OkLatency  int64
	States     map[string]int64
	mutex      sync.RWMutex
}

func (s *stats) IncrErr(start time.Time) {
	s.mutex.Lock()
	s.Err++
	s.Total++
	s.ErrLatency += int64(time.Since(start).Milliseconds())
	s.mutex.Unlock()
}

func (s *stats) IncrState(state string, start time.Time) {
	s.mutex.Lock()
	s.States[state]++
	s.Total++
	s.OkLatency += int64(time.Since(start).Milliseconds())
	s.mutex.Unlock()
}

func (s *stats) marshal() string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	d, _ := json.Marshal(s)
	return string(d)
}

func (s *stats) qps() float64 {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	ss := time.Since(s.Start).Seconds()
	return float64(s.Total) / ss
}

func (s *stats) latency() (int64, int64) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	l1, l2 := int64(0), int64(0)
	if s.Err != 0 {
		l1 = s.ErrLatency / s.Err
	}
	if s.Total-s.Err != 0 {
		l2 = s.OkLatency / (s.Total - s.Err)
	}
	return l1, l2
}

func ap() {
	stat := stats{
		States: make(map[string]int64),
		Start:  time.Now(),
	}

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
		wage := rand.Intn(100)
		rr, _ := constructRecords(wage)
		start := time.Now()
		resp, err := app.PayWage(rr)

		if err != nil {
			stat.IncrErr(start)
			fmt.Println(err)
		} else {
			stat.IncrState(resp.State, start)
		}
	}

	for i := 0; i < config.Ap.Concurrency; i++ {
		go func() {
			for {
				consume()
			}
		}()
	}

	for {
		<-time.After(2 * time.Second)
		e, o := stat.latency()
		logutil.Logger(context.Background()).Sugar().Infof("stats : %s, QPS(%v), Err(%v ms), Ok(%v ms)",
			stat.marshal(), stat.qps(), e, o)
	}
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
