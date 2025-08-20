// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package wireguard

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestWiregurad(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "pkg/cni/wireguard")
}
