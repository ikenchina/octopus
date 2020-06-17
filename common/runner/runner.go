package runner

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"

	_ "go.uber.org/automaxprocs"
	"go.uber.org/zap"

	"github.com/ikenchina/octopus/common/errorutil"
	logutil "github.com/ikenchina/octopus/common/log"
)

type Service interface {
	Start() error
	Stop() error
}

type ServiceRunner interface {
	Wait()
}

func RunService(s Service) ServiceRunner {
	r := newServiceRunner(s)
	r.run()
	return r
}

func RunHanlder(start func(), stop func()) {
	hs := &handlerService{
		start: start,
		stop:  stop,
	}
	RunService(hs).Wait()
}

type handlerService struct {
	start func()
	stop  func()
}

func (hs *handlerService) Start() error {
	hs.start()
	return nil
}
func (hs *handlerService) Stop() error {
	hs.stop()
	return nil
}

func newServiceRunner(s Service) *serviceRunner {
	return &serviceRunner{
		signals: make(chan os.Signal, 1),
		service: s,
	}
}

type serviceRunner struct {
	signals chan os.Signal
	service Service

	stopped int32

	wg sync.WaitGroup
}

func (r *serviceRunner) run() {
	r.wg.Add(1)
	go r.handleSignal()
	go r.handleStart()
}

func (r *serviceRunner) handleStart() {
	func() {
		defer errorutil.Recovery()
		err := r.service.Start()
		if err != nil {
			logutil.Logger(context.Background()).Fatal(err.Error())
		}
	}()
	if atomic.LoadInt32(&r.stopped) == 0 {
		r.wg.Done()
	}
}

func (r *serviceRunner) handleSignal() {
	signal.Notify(r.signals, syscall.SIGPIPE, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGABRT)
	for sig := range r.signals {
		logutil.Logger(context.Background()).Info("received ", zap.String("signal", sig.String()))
		switch sig {
		case syscall.SIGPIPE:
		case syscall.SIGINT:
			r.signalHandler()
			logutil.Logger(context.Background()).Info("Failure exit for systemd restarting")
			os.Exit(1)
		default:
			r.signalHandler()
			r.wg.Done()
		}
	}
}

func (r *serviceRunner) signalHandler() {

	atomic.StoreInt32(&r.stopped, 1)
	r.service.Stop()
}

func (r *serviceRunner) Wait() {
	r.wg.Wait()
	logutil.Sync()
}
