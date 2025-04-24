package main

import (
	"fmt"
	"os"

	"github.com/containernetworking/plugins/pkg/ns"

	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/netnswrapper"
)

func ensureNS(nsKit netnswrapper.Interface, namespace string) error {
	targetNS, err := nsKit.GetNS(namespace)
	if err != nil {
		if _, ok := err.(ns.NSPathNotExistErr); ok {
			fmt.Printf("Creating new network namespace %q\n", namespace)
			targetNS, err = nsKit.NewNS(namespace)
			if err != nil {
				return fmt.Errorf("failed to create network namespace %q: %w", namespace, err)
			}
			fmt.Printf("Created new network namespace %q\n", namespace)
		} else {
			return fmt.Errorf("failed to get network namespace %q: %w", namespace, err)
		}
	}
	defer targetNS.Close()
	return nil
}

func main() {
	nsKit := netnswrapper.NewNetNS()
	err := ensureNS(nsKit, consts.GatewayNetnsName)
	if err != nil {
		fmt.Println("Error:", err.Error())
		os.Exit(1)
	}
}
