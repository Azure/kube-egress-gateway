// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package v1alpha1

import "testing"

func TestValidateSGC(t *testing.T) {
	tests := map[string]struct {
		GatewayNodepoolName string
		VmssResourceGroup   string
		VmssName            string
		PublicIpPrefixSize  int32
		ProvisionPublicIps  bool
		PublicIpPrefixId    string
		ExpectErr           bool
	}{
		"It should pass when only gateway nodepool name is provided": {
			GatewayNodepoolName: "test",
		},
		"It should pass when vmss profile is provided": {
			VmssResourceGroup:  "test",
			VmssName:           "test",
			PublicIpPrefixSize: 31,
		},
		"It should fail when both gateway nodepool name and vmss profile are provided": {
			GatewayNodepoolName: "test",
			VmssResourceGroup:   "test",
			ExpectErr:           true,
		},
		"It should fail when no gateway profile is provided": {
			ExpectErr: true,
		},
		"It should fail when vmss resource group is not provided": {
			VmssName:           "test",
			PublicIpPrefixSize: 31,
			ExpectErr:          true,
		},
		"It should fail when vmss name is not provided": {
			VmssResourceGroup:  "test",
			PublicIpPrefixSize: 31,
			ExpectErr:          true,
		},
		"It should fail when PublicIpPrefixSize < 0": {
			VmssResourceGroup:  "test",
			VmssName:           "test",
			PublicIpPrefixSize: -1,
			ExpectErr:          true,
		},
		"It should fail when PublicIpPrefixSize > 31": {
			VmssResourceGroup:  "test",
			VmssName:           "test",
			PublicIpPrefixSize: 32,
			ExpectErr:          true,
		},
		"It should fail when PublicIPPrefixId is provided but ProvisionPublicIps is false": {
			GatewayNodepoolName: "test",
			ProvisionPublicIps:  false,
			PublicIpPrefixId:    "test",
			ExpectErr:           true,
		},
		"It should pass when PublicIPPrefixId is provided and ProvisionPublicIps is true": {
			GatewayNodepoolName: "test",
			ProvisionPublicIps:  true,
			PublicIpPrefixId:    "test",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sgw := &StaticGatewayConfiguration{
				Spec: StaticGatewayConfigurationSpec{
					GatewayNodepoolName: test.GatewayNodepoolName,
					GatewayVmssProfile: GatewayVmssProfile{
						VmssResourceGroup:  test.VmssResourceGroup,
						VmssName:           test.VmssName,
						PublicIpPrefixSize: test.PublicIpPrefixSize,
					},
					ProvisionPublicIps: test.ProvisionPublicIps,
					PublicIpPrefixId:   test.PublicIpPrefixId,
				},
			}
			err := sgw.validateSGC()
			if !test.ExpectErr && err != nil {
				t.Fatalf("failed to test validateSGC: expected no error: actual fail with err(%s)", err)
			}

			if test.ExpectErr && err == nil {
				t.Fatal("failed to test validateSGC: expected error: actual succeeded")
			}
		})
	}
}
