package main

import (
	"context"
	"flag"
	"os"

	"github.com/ikenchina/octopus/common/errorutil"
	logutil "github.com/ikenchina/octopus/common/log"
	"github.com/ikenchina/octopus/common/runner"
	"github.com/ikenchina/octopus/tc/config"
	tc "github.com/ikenchina/octopus/tc/service"
)

var (
	configFile = flag.String("config", "", "config file path")
)

func main() {
	flag.Parse()
	errorutil.PanicIfError(config.InitConfig(*configFile))
	svr := tc.NewTc()

	go func() {
		runner.SignalHandle(func(s os.Signal) bool {
			logutil.Logger(context.Background()).Sugar().Infof("received signal : %s.", s.String())
			svr.Stop()
			return true
		})
	}()

	svr.Start()
}
