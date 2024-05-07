// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package gcp

import (
	"context"
	"encoding/json"
	"strings"

	"cloud.google.com/go/compute/metadata"
)

const (
	gcpResolverServer = "169.254.169.254"
	gcpTimeServer     = "metadata.google.internal"
)

// MetadataConfig holds meta info.
type MetadataConfig struct {
	ProjectID    string `json:"project-id"`
	Name         string `json:"name,omitempty"`
	Hostname     string `json:"hostname,omitempty"`
	Zone         string `json:"zone,omitempty"`
	InstanceType string `json:"machine-type"`
	InstanceID   string `json:"id"`
	Preempted    string `json:"preempted"`
}

// NetworkInterfaceConfig holds network meta info.
type NetworkInterfaceConfig struct {
	AccessConfigs []struct {
		ExternalIP string `json:"externalIp,omitempty"`
		Type       string `json:"type,omitempty"`
	} `json:"accessConfigs,omitempty"`
	GatewayIPv4 string   `json:"gateway,omitempty"`
	GatewayIPv6 string   `json:"gatewayIpv6,omitempty"`
	IPv4        string   `json:"ip,omitempty"`
	IPv6        []string `json:"ipv6,omitempty"`
	MTU         int      `json:"mtu,omitempty"`
}

func (g *GCP) getMetadata(context.Context) (*MetadataConfig, error) {
	var (
		meta MetadataConfig
		err  error
	)

	if meta.ProjectID, err = metadata.ProjectID(); err != nil {
		return nil, err
	}

	if meta.Name, err = metadata.InstanceName(); err != nil {
		return nil, err
	}

	instanceType, err := metadata.Get("instance/machine-type")
	if err != nil {
		return nil, err
	}

	meta.InstanceType = strings.TrimSpace(instanceType[strings.LastIndex(instanceType, "/")+1:])

	if meta.InstanceID, err = metadata.InstanceID(); err != nil {
		return nil, err
	}

	if meta.Hostname, err = metadata.Hostname(); err != nil {
		return nil, err
	}

	if meta.Zone, err = metadata.Zone(); err != nil {
		return nil, err
	}

	meta.Preempted, err = metadata.Get("instance/scheduling/preemptible")
	if err != nil {
		return nil, err
	}

	return &meta, nil
}

func (g *GCP) getNetworkMetadata(context.Context) ([]NetworkInterfaceConfig, error) {
	metadataNetworkConfigDl, err := metadata.Get("instance/network-interfaces/?recursive=true&alt=json")
	if err != nil {
		return nil, err
	}

	var unmarshalledNetworkConfig []NetworkInterfaceConfig

	if err = json.Unmarshal([]byte(metadataNetworkConfigDl), &unmarshalledNetworkConfig); err != nil {
		return nil, err
	}

	return unmarshalledNetworkConfig, nil
}
