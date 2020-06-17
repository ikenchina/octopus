package errorutil

import (
	"context"

	logutil "github.com/ikenchina/octopus/common/log"
	"go.uber.org/zap"
)

func PanicIfError(err error) {
	if err == nil {
		return
	}
	logutil.Logger(context.Background()).Fatal("panic : ", zap.Error(err))
	logutil.Sync()
	panic(err)
}
