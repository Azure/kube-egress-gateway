package logger

import (
	"fmt"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
)

var defaultLogger logr.Logger

// SetLogger sets the default logger
func SetDefaultLogger(logger logr.Logger) {
	defaultLogger = logger
}

// GetLogger returns the default logger
func GetLogger() logr.Logger {
	return defaultLogger
}

func init() {
	zapLog, err := zap.NewProduction()
	if err != nil {
		panic(fmt.Sprintf("who watches the watchmen (%v)?", err))
	}
	SetDefaultLogger(zapr.NewLogger(zapLog))
}
