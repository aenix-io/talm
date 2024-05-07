// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package exoscale contains the Exoscale platform implementation.
package exoscale

import (
	"context"
	"fmt"
	"log"
	"net/netip"
	"strings"

	"github.com/cosi-project/runtime/pkg/state"
	"github.com/siderolabs/go-procfs/procfs"

	"github.com/aenix-io/talm/internal/app/machined/pkg/runtime"
	"github.com/aenix-io/talm/internal/app/machined/pkg/runtime/v1alpha1/platform/errors"
	"github.com/aenix-io/talm/internal/app/machined/pkg/runtime/v1alpha1/platform/internal/netutils"
	"github.com/siderolabs/talos/pkg/download"
	"github.com/siderolabs/talos/pkg/machinery/constants"
	"github.com/siderolabs/talos/pkg/machinery/resources/network"
	runtimeres "github.com/siderolabs/talos/pkg/machinery/resources/runtime"
)

// Exoscale is the concrete type that implements the runtime.Platform interface.
type Exoscale struct{}

// ParseMetadata converts Exoscale platform metadata into platform network config.
func (e *Exoscale) ParseMetadata(metadata *MetadataConfig) (*runtime.PlatformNetworkConfig, error) {
	networkConfig := &runtime.PlatformNetworkConfig{}

	if metadata.Hostname != "" {
		hostnameSpec := network.HostnameSpecSpec{
			ConfigLayer: network.ConfigPlatform,
		}

		if err := hostnameSpec.ParseFQDN(metadata.Hostname); err != nil {
			return nil, err
		}

		networkConfig.Hostnames = append(networkConfig.Hostnames, hostnameSpec)
	}

	if metadata.PublicIPv4 != "" {
		if ip, err := netip.ParseAddr(metadata.PublicIPv4); err == nil {
			networkConfig.ExternalIPs = append(networkConfig.ExternalIPs, ip)
		}
	}

	networkConfig.Metadata = &runtimeres.PlatformMetadataSpec{
		Platform:     e.Name(),
		Hostname:     metadata.Hostname,
		Region:       metadata.Zone,
		Zone:         metadata.Zone,
		InstanceType: strings.ToLower(strings.SplitN(metadata.InstanceType, " ", 2)[0]),
		InstanceID:   metadata.InstanceID,
		ProviderID:   fmt.Sprintf("exoscale://%s", metadata.InstanceID),
	}

	return networkConfig, nil
}

// Name implements the runtime.Platform interface.
func (e *Exoscale) Name() string {
	return "exoscale"
}

// Configuration implements the runtime.Platform interface.
func (e *Exoscale) Configuration(ctx context.Context, r state.State) ([]byte, error) {
	if err := netutils.Wait(ctx, r); err != nil {
		return nil, err
	}

	log.Printf("fetching machine config from %q", ExoscaleUserDataEndpoint)

	return download.Download(ctx, ExoscaleUserDataEndpoint,
		download.WithErrorOnNotFound(errors.ErrNoConfigSource),
		download.WithErrorOnEmptyResponse(errors.ErrNoConfigSource))
}

// Mode implements the runtime.Platform interface.
func (e *Exoscale) Mode() runtime.Mode {
	return runtime.ModeCloud
}

// KernelArgs implements the runtime.Platform interface.
func (e *Exoscale) KernelArgs(string) procfs.Parameters {
	return []*procfs.Parameter{
		procfs.NewParameter("console").Append("tty1").Append("ttyS0"),
		procfs.NewParameter(constants.KernelParamNetIfnames).Append("0"),
	}
}

// NetworkConfiguration implements the runtime.Platform interface.
func (e *Exoscale) NetworkConfiguration(ctx context.Context, _ state.State, ch chan<- *runtime.PlatformNetworkConfig) error {
	log.Printf("fetching exoscale instance config from: %q", ExoscaleMetadataEndpoint)

	metadata, err := e.getMetadata(ctx)
	if err != nil {
		return err
	}

	networkConfig, err := e.ParseMetadata(metadata)
	if err != nil {
		return err
	}

	select {
	case ch <- networkConfig:
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}
