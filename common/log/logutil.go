package logutil

import (
	"context"

	"go.uber.org/zap"
)

type ctxKeyType int

const ctxLogKey ctxKeyType = iota

var (
	_defaultLogger = newDefaultStdLogger()
	_globalLogger  = _defaultLogger
)

func SetLogger(l *zap.Logger) {
	_globalLogger = l
}

func Logger(ctx context.Context) *zap.Logger {
	if ctxlogger, ok := ctx.Value(ctxLogKey).(*zap.Logger); ok {
		return ctxlogger
	}
	return _globalLogger
}

func Sync() error {
	return _globalLogger.Sync()
}

func newDefaultStdLogger() *zap.Logger {
	lg, _ := zap.NewProduction()
	return lg
}
