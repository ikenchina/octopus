package logutil

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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

func InitLog(cfg *zap.Config) error {
	cfg.EncoderConfig.TimeKey = "time"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.EncoderConfig.CallerKey = "caller"
	cfg.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	cfg.EncoderConfig.StacktraceKey = "stacktrace"
	cfg.EncoderConfig.LineEnding = zapcore.DefaultLineEnding
	cfg.EncoderConfig.EncodeDuration = zapcore.SecondsDurationEncoder
	cfg.EncoderConfig.EncodeLevel = zapcore.LowercaseLevelEncoder

	logger, err := cfg.Build()
	if err != nil {
		return err
	}

	SetLogger(logger)
	return nil
}
