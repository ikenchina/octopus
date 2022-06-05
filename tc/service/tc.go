package service

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/ikenchina/octopus/common/errorutil"
	"github.com/ikenchina/octopus/common/idgenerator"
	logutil "github.com/ikenchina/octopus/common/log"
	"github.com/ikenchina/octopus/common/metrics"
	"github.com/ikenchina/octopus/tc/config"
	saga "github.com/ikenchina/octopus/tc/service/saga"
	tcc "github.com/ikenchina/octopus/tc/service/tcc"
)

type TcService struct {
	idGenerator idgenerator.IdGenerator
	httpServer  *http.Server
	sagaService *saga.SagaService
	tccService  *tcc.TccService
	isClose     int32
}

func equalPanic(e bool, msg string) {
	if e {
		logutil.Logger(context.Background()).Fatal(msg)
	}
}

func NewTc() *TcService {
	tc := &TcService{}
	var err error
	cfg := config.Get()

	lessee := fmt.Sprintf("%d_%d", cfg.Node.DataCenterId, cfg.Node.NodeId)
	tc.idGenerator, err = idgenerator.NewSnowflake(int64(cfg.Node.NodeId), int64(cfg.Node.DataCenterId))
	errorutil.PanicIfError(err)

	// saga
	sagaCfg, ok := cfg.Storages["saga"]
	equalPanic(!ok, "saga storage configuration does not exist")
	tc.sagaService, err = saga.NewSagaService(saga.Config{
		NodeId:              cfg.Node.NodeId,
		DataCenterId:        cfg.Node.DataCenterId,
		Store:               sagaCfg,
		MaxConcurrentTask:   cfg.MaxConcurrentTask,
		MaxConcurrentBranch: cfg.MaxConcurrentBranch,
		Lessee:              lessee,
	})
	errorutil.PanicIfError(err)

	// tcc
	tccCfg, ok := cfg.Storages["tcc"]
	equalPanic(!ok, "tcc storage configuration does not exist")
	tc.tccService, err = tcc.NewTccService(tcc.Config{
		NodeId:              cfg.Node.NodeId,
		DataCenterId:        cfg.Node.DataCenterId,
		Store:               tccCfg,
		MaxConcurrentTask:   cfg.MaxConcurrentTask,
		MaxConcurrentBranch: cfg.MaxConcurrentBranch,
	})
	errorutil.PanicIfError(err)

	// http server
	gin.SetMode(gin.ReleaseMode)
	app := gin.New()
	app.NoRoute(func(c *gin.Context) {
		c.JSON(404, gin.H{"code": "NOT_FOUND", "message": "not found"})
	})

	app.Use(func(c *gin.Context) {
		timer := httpHandleTimer.Timer()
		c.Next()
		timer(c.FullPath(), c.Request.Method, strconv.Itoa(c.Writer.Status()))
	})

	app.GET("/debug/healthcheck", tc.HealthCheck)
	app.GET("/debug/metrics", gin.WrapH(promhttp.Handler()))
	pprof.Register(app, "debug/pprof")

	// saga
	sagaGroup := app.Group("/dtx/saga")
	sagaGroup.GET("/gtid", tc.NewGtid)
	sagaGroup.POST("", tc.sagaService.Commit)
	sagaGroup.GET("/:gtid", tc.sagaService.Get)

	// tcc
	tccGroup := app.Group("/dtx/tcc")
	tccGroup.GET("/gtid", tc.NewGtid)
	tccGroup.POST("", tc.tccService.Prepare)
	tccGroup.POST("/:gtid", tc.tccService.Register)
	tccGroup.PUT("/:gtid", tc.tccService.Confirm)
	tccGroup.DELETE("/:gtid", tc.tccService.Cancel)
	tccGroup.GET("/:gtid", tc.tccService.Get)

	tc.httpServer = &http.Server{
		Addr:    cfg.HttpListen,
		Handler: app,
	}
	return tc
}

func (tc *TcService) Start() {
	logutil.Logger(context.Background()).Info("start service...")
	errorutil.PanicIfError(tc.sagaService.Start())
	errorutil.PanicIfError(tc.tccService.Start())

	logutil.Logger(context.Background()).Info("start http server")
	err := tc.httpServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		errorutil.PanicIfError(err)
	}
}

func (tc *TcService) Stop() {
	if atomic.CompareAndSwapInt32(&tc.isClose, 0, 1) {
		tc.stop()
	}
}

func (tc *TcService) stop() {
	log := func(msg string, err error) {
		if err != nil {
			logutil.Logger(context.Background()).Sugar().Errorf(msg+", error(%v)", err)
		} else {
			logutil.Logger(context.Background()).Sugar().Info(msg)
		}
	}

	// maximum time for below snippet including service stop
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	// stop service first, ensure outstanding requests are completed gracefully,
	// ingoing requests will be refused.
	log("stop saga", tc.sagaService.Stop())
	log("stop tcc", tc.tccService.Stop())
	log("stop http server", tc.httpServer.Shutdown(ctx))
	logutil.Sync()
}

func (tc *TcService) NewGtid(c *gin.Context) {
	id, err := tc.idGenerator.NextId()
	if err != nil {
		c.Status(500)
		_, _ = c.Writer.Write([]byte(err.Error()))
	}
	logutil.Logger(c.Request.Context()).Debug("new gtid", zap.Int64("id", id))
	c.JSON(200, map[string]string{"gtid": strconv.FormatInt(id, 10)})
}

// RESTful APIs

func (tc *TcService) HealthCheck(c *gin.Context) {
	c.Status(200)
}

var (
	httpHandleTimer = metrics.NewTimer("dtx", "http_server", "http handler metrics", []string{"path", "method", "code"})
)
