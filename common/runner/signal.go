package runner

import (
	"os"
	"os/signal"
	"syscall"
)

func SignalHandle(handleFunc func(os.Signal) bool) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGPIPE, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGABRT)

	for sig := range signals {
		if handleFunc == nil {
			return
		}
		if handleFunc(sig) {
			return
		}
	}
}
