package errorutil

import (
	"context"
	"runtime"

	logutil "github.com/ikenchina/octopus/common/log"
)

func Recovery(funcs ...RecoveryFallBackFunc) {
	if r := recover(); r != nil {
		recovered := false
		if len(funcs) > 0 {
			for _, fun := range funcs {
				if fun != nil {
					fun(r)
					recovered = true
				}
			}
		}
		if !recovered {
			buf := make([]byte, 1<<18)
			n := runtime.Stack(buf, false)
			logutil.Logger(context.Background()).Sugar().Errorf("%v, STACK: %s", r, buf[0:n])
		}
	}
}

type RecoveryFallBackFunc func(interface{})
