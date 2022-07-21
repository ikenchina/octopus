package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/ikenchina/octopus/common/errorutil"
	logutil "github.com/ikenchina/octopus/common/log"
	rmcommon "github.com/ikenchina/octopus/rm/common"
	"github.com/ikenchina/octopus/test/ap"
	saga_rm "github.com/ikenchina/octopus/test/rm/saga"
	tcc_rm "github.com/ikenchina/octopus/test/rm/tcc"
)

var (
	role      = flag.String("role", "ap", "role")
	portStart = flag.Int("port", 10001, "port start")
	userCount = flag.Int("user_count", 2, "user count")
	txnType   = flag.String("txn", "saga", "type")
)

func main() {
	flag.Parse()
	logger, _ := zap.NewDevelopment()
	logutil.SetLogger(logger)
	gin.SetMode(gin.DebugMode)
	rmcommon.SetDbSchema("dtx")

	switch *role {
	case "rm":
		initRm()
	case "ap":
		runAp()
	default:
		initRm()
		runAp()
	}
}

func runAp() {
	if *txnType == "saga" {
		paywage()
	} else {
		transfer()
	}
}

func paywage() {
	app := initSagaAp()
	resp, err := app.Pay(constructSagaRecords())
	logutil.Logger(context.Background()).Sugar().Debugf("pay wages : %v %v", resp, err)
	time.Sleep(time.Second * 1)
}

func transfer() {
	app := initTccAp()
	resp, err := app.Transfer(constructTccRecords())
	logutil.Logger(context.Background()).Sugar().Debugf("transfer : %v %v", resp, err)
	time.Sleep(time.Second * 1)
}

func constructSagaRecords() []*saga_rm.BankAccountRecord {
	records := []*saga_rm.BankAccountRecord{}
	wage := 10
	//fail := rand.Intn(10) < 5
	for i := 0; i < *userCount; i++ {
		if i == 0 {
			records = append(records, &saga_rm.BankAccountRecord{
				UserID:  i, // account of company
				Account: -1 * (*userCount - 1) * wage,
			})
		} else { // employees' account
			records = append(records, &saga_rm.BankAccountRecord{
				UserID:  i,
				Account: wage,
				//Fail:    fail,
			})
		}
	}
	return records
}

func constructTccRecords() []*tcc_rm.BankAccountRecord {
	records := []*tcc_rm.BankAccountRecord{}
	wage := 10
	//fail := rand.Intn(10) < 5
	for i := 0; i < *userCount; i++ {
		if i == 0 {
			records = append(records, &tcc_rm.BankAccountRecord{
				UserID:  i, // account of company
				Account: -1 * (*userCount - 1) * wage,
			})
		} else { // employees' account
			records = append(records, &tcc_rm.BankAccountRecord{
				UserID:  i,
				Account: wage,
				//Fail:    fail,
			})
		}
	}
	return records
}

func initBankHosts() []ap.Bank {
	hosts := []ap.Bank{}
	for i := 0; i < *userCount; i++ {
		port := *portStart + i
		host := ap.Bank{
			Target:   fmt.Sprintf("http://localhost:%d", port),
			Protocol: "http",
			Port:     strconv.Itoa(port),
		}
		//if i%2 == 0 { // grpc
		host.Target = fmt.Sprintf("localhost:%d", port)
		host.Protocol = "grpc"
		host.Port = strconv.Itoa(port)
		//}
		hosts = append(hosts, host)
	}
	return hosts
}

func initRm() {
	hosts := initBankHosts()
	wait := sync.WaitGroup{}
	wait.Add(*userCount)
	db := initDb("postgresql://dtx_user:dtx_pass@127.0.0.1:5432/dtx?connect_timeout=3")

	for i := 0; i < len(hosts); i++ {
		host := hosts[i]
		go func() {
			defer wait.Done()
			if *txnType == "saga" {
				errorutil.PanicIfError(saga_rm.StartRm(fmt.Sprintf(":%v", host.Port), db, host.Protocol))
			} else {
				errorutil.PanicIfError(tcc_rm.StartRm(fmt.Sprintf(":%v", host.Port), db, host.Protocol))
			}
		}()
	}
	wait.Wait()
}

func initSagaAp() *ap.SagaApplication {
	app := &ap.SagaApplication{}

	tcListen := ":18080"
	apListen := ":17080"
	tcDomain := fmt.Sprintf("localhost%s", tcListen)
	hosts := initBankHosts()
	for _, host := range hosts {
		app.AddBank(host)
	}
	app.InitTcClient(tcDomain)
	go func() {
		err := app.InitHttp(apListen)
		if err != nil {
			log.Panic(err)
		}
	}()
	return app
}

func initTccAp() *ap.TccApplication {
	app := &ap.TccApplication{}

	tcListen := ":18080"
	apListen := ":17080"
	tcDomain := fmt.Sprintf("localhost%s", tcListen)
	hosts := initBankHosts()
	for _, host := range hosts {
		app.AddBank(host)
	}
	app.InitTcClient(tcDomain)
	go func() {
		err := app.InitHttp(apListen)
		if err != nil {
			log.Panic(err)
		}
	}()
	return app
}

func initDb(dsn string) *gorm.DB {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	errorutil.PanicIfError(err)
	return db
}
