// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package cnimanager_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCnimanager(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cnimanager Suite")
}
