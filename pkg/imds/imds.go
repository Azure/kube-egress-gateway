// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package imds

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	// ImdsInstanceAPIVersion is the imds instance api version
	ImdsInstanceAPIVersion = "2021-10-01"
	// ImdsLoadBalancerAPIVersion is the imds load balancer api version
	ImdsLoadBalancerAPIVersion = "2020-10-01"
	// ImdsServer is the imds server endpoint
	ImdsServer = "http://169.254.169.254"
	// ImdsInstanceURI is the imds instance uri
	ImdsInstanceURI = "/metadata/instance"
	// ImdsLoadBalancerURI is the imds load balancer uri
	ImdsLoadBalancerURI = "/metadata/loadbalancer"
	// ImdsUserAgent is the user agent to query Imds
	ImdsUserAgent = "golang/kube-egress-gateway"
)

func GetInstanceMetadata() (*InstanceMetadata, error) {
	data, err := getImdsResponse(ImdsInstanceURI, ImdsInstanceAPIVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance metadata: %w", err)
	}
	obj := InstanceMetadata{}
	err = json.Unmarshal(data, &obj)
	if err != nil {
		return nil, err
	}

	return &obj, nil
}

func GetLoadBalancerMetadata() (*LoadBalancerMetadata, error) {
	data, err := getImdsResponse(ImdsLoadBalancerURI, ImdsLoadBalancerAPIVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to get loadbalancer metadata: %w", err)
	}
	obj := LoadBalancerMetadata{}
	err = json.Unmarshal(data, &obj)
	if err != nil {
		return nil, err
	}

	return &obj, nil
}

func getImdsResponse(resourceURI, apiVersion string) ([]byte, error) {
	req, err := http.NewRequest("GET", ImdsServer+resourceURI, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Metadata", "True")
	req.Header.Add("User-Agent", ImdsUserAgent)

	q := req.URL.Query()
	q.Add("format", "json")
	q.Add("api-version", apiVersion)
	req.URL.RawQuery = q.Encode()

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failure of querying imds with response %q", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return data, nil
}
