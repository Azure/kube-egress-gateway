// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package conf

import (
	"reflect"
	"testing"

	"github.com/containernetworking/cni/pkg/types"
)

func TestParseCNIConfig(t *testing.T) {
	tests := map[string]struct {
		StdinData string
		Expected  *CNIConfig
	}{
		"test cni config without prevResult": {
			StdinData: `{"cniVersion":"1.0.0","excludedCIDRs":["1.2.3.4/32","10.1.0.0/16"],"socketPath":"testPath","gatewayName":"test","ipam":{"type":"kube-egress-cni-ipam"},"name":"mynet","type":"kube-egress-cni"}`,
			Expected: &CNIConfig{
				NetConf: types.NetConf{
					CNIVersion: "1.0.0",
					Name:       "mynet",
					Type:       "kube-egress-cni",
					IPAM:       types.IPAM{Type: "kube-egress-cni-ipam"},
				},
				ExcludedCIDRs: []string{"1.2.3.4/32", "10.1.0.0/16"},
				SocketPath:    "testPath",
			},
		},
		"test cni config with preResult": {
			StdinData: `{"cniVersion":"1.0.0","excludedCIDRs":[],"socketPath":"testPath","gatewayName":"test","ipam":{"type":"kube-egress-cni-ipam"},"name":"mynet","prevResult":{"cniVersion":"1.0.0","interfaces":[{"name":"wg0","sandbox":"somepath"}],"ips":[{"interface":0,"address":"fe80::1/64"},{"address":"10.2.0.1/24"}],"dns":{}},"type":"kube-egress-cni"}`,
			Expected: &CNIConfig{
				NetConf: types.NetConf{
					CNIVersion: "1.0.0",
					Name:       "mynet",
					Type:       "kube-egress-cni",
					IPAM:       types.IPAM{Type: "kube-egress-cni-ipam"},
				},
				ExcludedCIDRs: []string{},
				SocketPath:    "testPath",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			res, err := ParseCNIConfig([]byte(test.StdinData))
			if err != nil {
				t.Fatalf("failed to parse CNI config: %v", err)
			}
			if !reflect.DeepEqual(test.Expected.ExcludedCIDRs, res.ExcludedCIDRs) ||
				test.Expected.CNIVersion != res.CNIVersion ||
				test.Expected.Name != res.Name ||
				test.Expected.Type != res.Type ||
				test.Expected.IPAM != res.IPAM {
				t.Fatalf("got different cniConfig from ParseCNIConfig, expected: %#v, got: %#v", *test.Expected, *res)
			}
		})
	}
}

func TestLoadK8sInfo(t *testing.T) {
	testArg := `IgnoreUnknown=true;K8S_POD_NAMESPACE=testns;K8S_POD_NAME=testpod;K8S_POD_INFRA_CONTAINER_ID=1234567890;K8S_POD_UID=12345678-1234-1234-1234-123456789012`
	expected := &K8sConfig{
		CommonArgs:                 types.CommonArgs{IgnoreUnknown: types.UnmarshallableBool(true)},
		K8S_POD_NAME:               types.UnmarshallableString("testpod"),
		K8S_POD_NAMESPACE:          types.UnmarshallableString("testns"),
		K8S_POD_INFRA_CONTAINER_ID: types.UnmarshallableString("1234567890"),
	}
	res, err := LoadK8sInfo(testArg)
	if err != nil {
		t.Fatalf("unexpected error when loading k8s information: %v", err)
	}
	if !reflect.DeepEqual(res, expected) {
		t.Fatalf("got different k8sConfig from LoadK8sInfo, expected: %#v, got: %#v", *expected, *res)
	}
}
