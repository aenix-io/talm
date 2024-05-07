// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.
package kubespan_test

import (
	"net/netip"
	"testing"
	"time"

	"github.com/cosi-project/runtime/pkg/resource"
	"github.com/siderolabs/go-retry/retry"
	"github.com/stretchr/testify/suite"

	kubespanctrl "github.com/aenix-io/talm/internal/app/machined/pkg/controllers/kubespan"
	"github.com/siderolabs/talos/pkg/machinery/config/machine"
	"github.com/siderolabs/talos/pkg/machinery/resources/cluster"
	"github.com/siderolabs/talos/pkg/machinery/resources/config"
	"github.com/siderolabs/talos/pkg/machinery/resources/kubespan"
)

type EndpointSuite struct {
	KubeSpanSuite
}

func (suite *EndpointSuite) TestReconcile() {
	suite.Require().NoError(suite.runtime.RegisterController(&kubespanctrl.EndpointController{}))

	suite.startRuntime()

	cfg := kubespan.NewConfig(config.NamespaceName, kubespan.ConfigID)
	cfg.TypedSpec().HarvestExtraEndpoints = true
	suite.Require().NoError(suite.state.Create(suite.ctx, cfg))

	// create some affiliates and peer statuses
	affiliate1 := cluster.NewAffiliate(cluster.NamespaceName, "7x1SuC8Ege5BGXdAfTEff5iQnlWZLfv9h1LGMxA2pYkC")
	*affiliate1.TypedSpec() = cluster.AffiliateSpec{
		NodeID:      "7x1SuC8Ege5BGXdAfTEff5iQnlWZLfv9h1LGMxA2pYkC",
		Hostname:    "foo.com",
		Nodename:    "bar",
		MachineType: machine.TypeControlPlane,
		Addresses:   []netip.Addr{netip.MustParseAddr("192.168.3.4")},
		KubeSpan: cluster.KubeSpanAffiliateSpec{
			PublicKey:           "PLPNBddmTgHJhtw0vxltq1ZBdPP9RNOEUd5JjJZzBRY=",
			Address:             netip.MustParseAddr("fd50:8d60:4238:6302:f857:23ff:fe21:d1e0"),
			AdditionalAddresses: []netip.Prefix{netip.MustParsePrefix("10.244.3.1/24")},
			Endpoints:           []netip.AddrPort{netip.MustParseAddrPort("10.0.0.2:51820"), netip.MustParseAddrPort("192.168.3.4:51820")},
		},
	}

	affiliate2 := cluster.NewAffiliate(cluster.NamespaceName, "roLng5hmP0Gv9S5Pbfzaa93JSZjsdpXNAn7vzuCfsc8")
	*affiliate2.TypedSpec() = cluster.AffiliateSpec{
		NodeID:      "roLng5hmP0Gv9S5Pbfzaa93JSZjsdpXNAn7vzuCfsc8",
		MachineType: machine.TypeControlPlane,
		Addresses:   []netip.Addr{netip.MustParseAddr("192.168.3.5")},
		KubeSpan: cluster.KubeSpanAffiliateSpec{
			PublicKey: "1CXkdhWBm58c36kTpchR8iGlXHG1ruHa5W8gsFqD8Qs=",
			Address:   netip.MustParseAddr("fd50:8d60:4238:6302:f857:23ff:fe21:d1e1"),
		},
	}

	suite.Require().NoError(suite.state.Create(suite.ctx, affiliate1))
	suite.Require().NoError(suite.state.Create(suite.ctx, affiliate2))

	peerStatus1 := kubespan.NewPeerStatus(kubespan.NamespaceName, affiliate1.TypedSpec().KubeSpan.PublicKey)
	*peerStatus1.TypedSpec() = kubespan.PeerStatusSpec{
		Endpoint: netip.MustParseAddrPort("10.3.4.8:278"),
		State:    kubespan.PeerStateUp,
	}

	peerStatus2 := kubespan.NewPeerStatus(kubespan.NamespaceName, affiliate2.TypedSpec().KubeSpan.PublicKey)
	*peerStatus2.TypedSpec() = kubespan.PeerStatusSpec{
		Endpoint: netip.MustParseAddrPort("10.3.4.9:279"),
		State:    kubespan.PeerStateUnknown,
	}

	peerStatus3 := kubespan.NewPeerStatus(kubespan.NamespaceName, "LoXPyyYh3kZwyKyWfCcf9VvgVv588cKhSKXavuUZqDg=")
	*peerStatus3.TypedSpec() = kubespan.PeerStatusSpec{
		Endpoint: netip.MustParseAddrPort("10.3.4.10:270"),
		State:    kubespan.PeerStateUp,
	}

	suite.Require().NoError(suite.state.Create(suite.ctx, peerStatus1))
	suite.Require().NoError(suite.state.Create(suite.ctx, peerStatus2))
	suite.Require().NoError(suite.state.Create(suite.ctx, peerStatus3))

	// peer1 is up and has matching affiliate
	suite.Assert().NoError(retry.Constant(3*time.Second, retry.WithUnits(100*time.Millisecond)).Retry(
		suite.assertResource(
			resource.NewMetadata(
				kubespan.NamespaceName,
				kubespan.EndpointType,
				peerStatus1.Metadata().ID(),
				resource.VersionUndefined,
			),
			func(res resource.Resource) error {
				spec := res.(*kubespan.Endpoint).TypedSpec()

				suite.Assert().Equal(peerStatus1.TypedSpec().Endpoint, spec.Endpoint)
				suite.Assert().Equal(affiliate1.TypedSpec().NodeID, spec.AffiliateID)

				return nil
			},
		),
	))

	// peer2 is not up, it shouldn't be published as an endpoint
	suite.Assert().NoError(retry.Constant(3*time.Second, retry.WithUnits(100*time.Millisecond)).Retry(
		suite.assertNoResource(
			resource.NewMetadata(
				kubespan.NamespaceName,
				kubespan.EndpointType,
				peerStatus2.Metadata().ID(),
				resource.VersionUndefined,
			),
		),
	))

	// peer3 is up, but has not matching affiliate
	suite.Assert().NoError(retry.Constant(3*time.Second, retry.WithUnits(100*time.Millisecond)).Retry(
		suite.assertNoResource(
			resource.NewMetadata(
				kubespan.NamespaceName,
				kubespan.EndpointType,
				peerStatus3.Metadata().ID(),
				resource.VersionUndefined,
			),
		),
	))
}

func TestEndpointSuite(t *testing.T) {
	t.Parallel()

	suite.Run(t, new(EndpointSuite))
}
