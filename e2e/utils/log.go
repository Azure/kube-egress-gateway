// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package utils

import (
	"fmt"
	"runtime"
	"time"

	"github.com/onsi/ginkgo/v2"
)

func log(level string, format string, args ...interface{}) {
	current := time.Now().Format(time.StampMilli)
	_, file, line, _ := runtime.Caller(2)
	extra := fmt.Sprintf(" [%s:%d]", file, line)

	_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, current+": "+level+": "+format+extra+"\n", args...)
}

// Logf prints info logs
func Logf(format string, args ...interface{}) {
	log("INFO", format, args...)
}
