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

package main

import (
	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	type100 "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/cni/pkg/version"

	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"
)

func main() {
	skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.All, bv.BuildString("none"))
}

func cmdAdd(args *skel.CmdArgs) error {
	// outputCmdArgs(args)
	netConf, _ := libcni.ConfFromBytes(args.StdinData)

	return types.PrintResult(getResult(netConf), netConf.Network.CNIVersion)
}

func cmdDel(args *skel.CmdArgs) error {
	// outputCmdArgs(args)
	netConf, _ := libcni.ConfFromBytes(args.StdinData)

	return types.PrintResult(&type100.Result{}, netConf.Network.CNIVersion)
}

func cmdCheck(args *skel.CmdArgs) error {
	// outputCmdArgs(args)
	netConf, _ := libcni.ConfFromBytes(args.StdinData)

	return types.PrintResult(&type100.Result{}, netConf.Network.CNIVersion)
}

// func outputCmdArgs(args *skel.CmdArgs) {
// 	fmt.Printf(`ContainerID: %s
// Netns: %s
// IfName: %s
// Args: %s
// Path: %s
// StdinData: %s
// ----------------------
// `,
// 		args.ContainerID,
// 		args.Netns,
// 		args.IfName,
// 		args.Args,
// 		args.Path,
// 		string(args.StdinData))
// }

func getResult(netConf *libcni.NetworkConfig) *type100.Result {
	if netConf.Network.RawPrevResult == nil {
		return &type100.Result{}
	}

	if err := version.ParsePrevResult(netConf.Network); err != nil {
		return &type100.Result{}
	}
	result, err := type100.NewResultFromResult(netConf.Network.PrevResult)
	if err != nil {
		return &type100.Result{}
	}
	return result
}
