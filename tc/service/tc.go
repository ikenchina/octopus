package service

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/ikenchina/octopus/common/errorutil"
	sgrpc "github.com/ikenchina/octopus/common/grpc"
	"github.com/ikenchina/octopus/common/idgenerator"
	logutil "github.com/ikenchina/octopus/common/log"
	"github.com/ikenchina/octopus/common/metrics"
	saga_pb "github.com/ikenchina/octopus/define/proto/saga/pb"
	tcc_pb "github.com/ikenchina/octopus/define/proto/tcc/pb"
	"github.com/ikenchina/octopus/tc/config"
	saga "github.com/ikenchina/octopus/tc/service/saga"
	tcc "github.com/ikenchina/octopus/tc/service/tcc"
)

type TcService struct {
	tcc_pb.UnimplementedTcServer
	idGenerator idgenerator.IdGenerator
	httpServer  *http.Server
	grpcServer  *grpc.Server
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
	}, tc.idGenerator)
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
		Lessee:              lessee,
	}, tc.idGenerator)
	errorutil.PanicIfError(err)

	return tc
}

func (tc *TcService) Start() {
	logutil.Logger(context.Background()).Info("start service...")

	errorutil.PanicIfError(sgrpc.Init(5000))
	errorutil.PanicIfError(tc.sagaService.Start())
	errorutil.PanicIfError(tc.tccService.Start())

	cfg := config.Get()

	wait := sync.WaitGroup{}
	if len(cfg.HttpListen) > 0 {
		wait.Add(1)
		go errorutil.SafeGoroutine(func() {
			defer wait.Done()
			err := tc.startHttpServer(cfg.HttpListen)
			if err != nil && err != http.ErrServerClosed {
				tc.stop()
			}
		})
	}

	if len(cfg.GrpcListen) > 0 {
		wait.Add(1)
		go errorutil.SafeGoroutine(func() {
			defer wait.Done()
			err := tc.startGrpcServer(cfg.GrpcListen)
			if err != nil && err != grpc.ErrServerStopped {
				tc.stop()
			}
		})
	}

	wait.Wait()
}

func (tc *TcService) Stop() {
	if atomic.CompareAndSwapInt32(&tc.isClose, 0, 1) {
		tc.stop()
	}
}

func (tc *TcService) startHttpServer(listen string) error {
	logutil.Logger(context.Background()).Sugar().Infof("start http server : listen(%v)", listen)

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

	app.Any("/debug/healthcheck", tc.HealthCheck)
	app.GET("/debug/metrics", gin.WrapH(promhttp.Handler()))
	app.Any("/debug/saga/:gtid", tc.sagaService.OpTxn)
	app.Any("/debug/tcc/:gtid", tc.tccService.OpTxn)

	pprof.Register(app, "debug/pprof")

	// saga
	sagaGroup := app.Group("/dtx/saga")
	sagaGroup.GET("/gtid", tc.NewGtid)
	sagaGroup.POST("", tc.sagaService.HttpCommit)
	sagaGroup.GET("/:gtid", tc.sagaService.HttpGet)

	// tcc
	tccGroup := app.Group("/dtx/tcc")
	tccGroup.GET("/gtid", tc.NewGtid)
	tccGroup.POST("", tc.tccService.HttpPrepare)
	tccGroup.POST("/:gtid", tc.tccService.HttpRegister)
	tccGroup.PUT("/:gtid", tc.tccService.HttpConfirm)
	tccGroup.DELETE("/:gtid", tc.tccService.HttpCancel)
	tccGroup.GET("/:gtid", tc.tccService.HttpGet)

	tc.httpServer = &http.Server{
		Addr:    listen,
		Handler: app,
	}

	return tc.httpServer.ListenAndServe()
}

func (tc *TcService) startGrpcServer(listen string) error {
	lis, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}

	sgrpc.RegisterCustomProto()

	tc.grpcServer = grpc.NewServer()
	saga_pb.RegisterTcServer(tc.grpcServer, tc.sagaService)
	tcc_pb.RegisterTcServer(tc.grpcServer, tc.tccService)
	reflection.Register(tc.grpcServer)

	logutil.Logger(context.Background()).Sugar().Infof("start grpc server : listen(%v)", listen)

	return tc.grpcServer.Serve(lis)
}

func (tc *TcService) stopHttpServer() error {
	if tc.httpServer != nil {
		// maximum time for below snippet including service stop
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		return tc.httpServer.Shutdown(ctx)
	}
	return nil
}

func (tc *TcService) stopGrpcServer() error {
	if tc.grpcServer != nil {
		tc.grpcServer.Stop()
	}
	return nil
}

func (tc *TcService) stop() {
	log := func(msg string, err error) {
		if err != nil {
			logutil.Logger(context.Background()).Sugar().Errorf(msg+", error(%v)", err)
		} else {
			logutil.Logger(context.Background()).Sugar().Info(msg)
		}
	}

	// stop service first, ensure outstanding requests are completed gracefully,
	// ingoing requests will be refused.
	log("stop saga", tc.sagaService.Stop())
	log("stop tcc", tc.tccService.Stop())
	log("stop http server", tc.stopHttpServer())
	log("stop grpc server", tc.stopGrpcServer())
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
	if c.Request.Method == http.MethodGet {
		c.Status(200)
	} else if c.Request.Method == http.MethodDelete {
		tc.Stop()
	}
}

var (
	httpHandleTimer = metrics.NewTimer("dtx", "http_server", "http handler metrics", []string{"path", "method", "code"})
)
