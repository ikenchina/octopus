package test

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ikenchina/octopus/common/errorutil"
	"github.com/ikenchina/octopus/tc/config"
)

var (
	configFile = flag.String("config", "./tc.json", "config file path")
)

type rmMock struct {
	listen         string
	ginApp         *gin.Engine
	tcDomain       string
	rmDomain       string
	id             int32
	servers        []rmServer
	httpServer     *http.Server
	basePath       string
	handlerFactory handlerFactory
}

type handlerFactory interface {
	notifyHandler() func(c *gin.Context)
	actionHandler(server int) func(c *gin.Context)
	commitHandler(server int) func(c *gin.Context)
	rollbackHandler(server int) func(c *gin.Context)
}

func newRm(basePath string, serverSize int, listen string, factory handlerFactory) *rmMock {
	rm := &rmMock{
		listen:         listen,
		ginApp:         gin.New(),
		handlerFactory: factory,
		basePath:       basePath,
	}

	cfg := config.Get()
	rm.tcDomain = fmt.Sprintf("http://localhost%s", cfg.HttpListen)
	rm.rmDomain = fmt.Sprintf("http://localhost%s", rm.listen)
	rm.httpServer = &http.Server{
		Addr:    rm.listen,
		Handler: rm.ginApp,
	}
	gin.SetMode(gin.DebugMode)

	rm.startRMs(serverSize)
	return rm
}

func (rm *rmMock) start() {
	err := rm.httpServer.ListenAndServe()
	if err != http.ErrServerClosed {
		panic(err)
	}
}

func (rm *rmMock) stop() {
	errorutil.PanicIfError(rm.httpServer.Shutdown(context.Background()))
}

func (rm *rmMock) newGtid() string {
	return fmt.Sprintf("gtid_%d", atomic.AddInt32(&rm.id, 1))
}

func (rm *rmMock) startRMs(rmSize int) error {
	rm.ginApp.POST(rm.basePath, rm.handlerFactory.notifyHandler())
	for i := 0; i < rmSize; i++ {
		svrPath := fmt.Sprintf("/service%d%s", i, rm.basePath)
		svr := rmServer{
			data:         make(map[string]*rmServerData),
			actionUrl:    rm.rmDomain + svrPath,
			commitUrl:    rm.rmDomain + svrPath,
			rollbackUrl:  rm.rmDomain + svrPath,
			commitChan:   make(chan string, 10),
			rollbackChan: make(chan string, 10),
			basePath:     svrPath,
		}
		rm.servers = append(rm.servers, svr)
		rm.ginApp.PUT(svrPath+"/:gtid/:branch_id", rm.handlerFactory.commitHandler(i))
		rm.ginApp.DELETE(svrPath+"/:gtid/:branch_id", rm.handlerFactory.rollbackHandler(i))
		rm.ginApp.POST(svrPath+"/:gtid/:branch_id", rm.handlerFactory.actionHandler(i))
	}
	return nil
}

type rmServer struct {
	basePath        string
	actionUrl       string
	commitUrl       string
	rollbackUrl     string
	actionErr       bool
	data            map[string]*rmServerData
	commitChan      chan string
	rollbackChan    chan string
	timeout         time.Duration
	actionTimeout   time.Duration
	rollbackTimeout time.Duration
}

type rmServerData struct {
	action   bool
	commit   bool
	rollback bool
}
