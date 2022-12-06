package conf

import (
	"encoding/json"
	"fmt"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/version"
)

type CNIConfig struct {
	types.NetConf

	ExcludedCIDRs      []string  `json:"excludedCIDRs"`
	GatewayName        string    `json:"gatewayName"`
	KeepAliveInSeconds int       `json:"keepAliveInSeconds"`
	K8sInfo            K8sConfig `json:"-"`
}

func ParseCNIConfig(stdin []byte) (*CNIConfig, error) {
	conf := &CNIConfig{}
	if err := json.Unmarshal(stdin, &conf); err != nil {
		return nil, fmt.Errorf("failed to parse network configuration: %v", err)
	}

	// Parse previous result. This will parse, validate, and place the
	// previous result object into conf.PrevResult. If you need to modify
	// or inspect the PrevResult you will need to convert it to a concrete
	// versioned Result struct.
	if err := version.ParsePrevResult(&conf.NetConf); err != nil {
		return nil, fmt.Errorf("could not parse prevResult: %v", err)
	}
	return conf, nil
}

// K8sArgs is the valid CNI_ARGS used for Kubernetes

type K8sConfig struct {
	types.CommonArgs
	K8S_POD_NAME               types.UnmarshallableString
	K8S_POD_NAMESPACE          types.UnmarshallableString
	K8S_POD_INFRA_CONTAINER_ID types.UnmarshallableString
}

func LoadK8sInfo(args string) (*K8sConfig, error) {
	k8sInfo := &K8sConfig{}
	if err := types.LoadArgs(args, k8sInfo); err != nil {
		return k8sInfo, err
	}
	return k8sInfo, nil
}
