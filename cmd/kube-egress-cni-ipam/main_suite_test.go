// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package main

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCNIIpamMain(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "cmd/kube-egress-cni-ipam")
}
