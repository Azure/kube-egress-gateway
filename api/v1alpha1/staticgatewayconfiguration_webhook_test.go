/*
   MIT License

   Copyright (c) Microsoft Corporation.

   Permission is hereby granted, free of charge, to any person obtaining a copy
   of this software and associated documentation files (the "Software"), to deal
   in the Software without restriction, including without limitation the rights
   to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
   copies of the Software, and to permit persons to whom the Software is
   furnished to do so, subject to the following conditions:

   The above copyright notice and this permission notice shall be included in all
   copies or substantial portions of the Software.

   THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
   IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
   FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
   AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
   LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
   OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
   SOFTWARE
*/

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
