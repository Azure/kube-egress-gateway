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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failure of querying imds with response %q", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return data, nil
}
