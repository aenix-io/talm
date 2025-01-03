// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

//nolint:dupl
package network_test

import (
	"context"
	"fmt"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/cosi-project/runtime/pkg/controller/runtime"
	"github.com/cosi-project/runtime/pkg/resource"
	"github.com/cosi-project/runtime/pkg/state"
	"github.com/cosi-project/runtime/pkg/state/impl/inmem"
	"github.com/cosi-project/runtime/pkg/state/impl/namespaced"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap/zaptest"

	netctrl "github.com/aenix-io/talm/internal/app/machined/pkg/controllers/network"
	"github.com/siderolabs/talos/pkg/machinery/constants"
	"github.com/siderolabs/talos/pkg/machinery/resources/network"
)

type ResolverMergeSuite struct {
	suite.Suite

	state state.State

	runtime *runtime.Runtime
	wg      sync.WaitGroup

	ctx       context.Context //nolint:containedctx
	ctxCancel context.CancelFunc
}

func (suite *ResolverMergeSuite) SetupTest() {
	suite.ctx, suite.ctxCancel = context.WithTimeout(context.Background(), 3*time.Minute)

	suite.state = state.WrapCore(namespaced.NewState(inmem.Build))

	var err error

	suite.runtime, err = runtime.NewRuntime(suite.state, zaptest.NewLogger(suite.T()))
	suite.Require().NoError(err)

	suite.Require().NoError(suite.runtime.RegisterController(&netctrl.ResolverMergeController{}))

	suite.startRuntime()
}

func (suite *ResolverMergeSuite) startRuntime() {
	suite.wg.Add(1)

	go func() {
		defer suite.wg.Done()

		suite.Assert().NoError(suite.runtime.Run(suite.ctx))
	}()
}

func (suite *ResolverMergeSuite) assertResolvers(requiredIDs []string, check func(*network.ResolverSpec, *assert.Assertions)) {
	assertResources(suite.ctx, suite.T(), suite.state, requiredIDs, check)
}

func (suite *ResolverMergeSuite) TestMerge() {
	def := network.NewResolverSpec(network.ConfigNamespaceName, "default/resolvers")
	*def.TypedSpec() = network.ResolverSpecSpec{
		DNSServers: []netip.Addr{
			netip.MustParseAddr(constants.DefaultPrimaryResolver),
			netip.MustParseAddr(constants.DefaultSecondaryResolver),
		},
		ConfigLayer: network.ConfigDefault,
	}

	dhcp1 := network.NewResolverSpec(network.ConfigNamespaceName, "dhcp/eth0")
	*dhcp1.TypedSpec() = network.ResolverSpecSpec{
		DNSServers:  []netip.Addr{netip.MustParseAddr("1.1.2.0")},
		ConfigLayer: network.ConfigOperator,
	}

	dhcp2 := network.NewResolverSpec(network.ConfigNamespaceName, "dhcp/eth1")
	*dhcp2.TypedSpec() = network.ResolverSpecSpec{
		DNSServers:  []netip.Addr{netip.MustParseAddr("1.1.2.1")},
		ConfigLayer: network.ConfigOperator,
	}

	static := network.NewResolverSpec(network.ConfigNamespaceName, "configuration/resolvers")
	*static.TypedSpec() = network.ResolverSpecSpec{
		DNSServers:    []netip.Addr{netip.MustParseAddr("2.2.2.2")},
		SearchDomains: []string{"example.com", "example.org", "example.net"},
		ConfigLayer:   network.ConfigMachineConfiguration,
	}

	for _, res := range []resource.Resource{def, dhcp1, dhcp2, static} {
		suite.Require().NoError(suite.state.Create(suite.ctx, res), "%v", res.Spec())
	}

	suite.assertResolvers(
		[]string{
			"resolvers",
		}, func(r *network.ResolverSpec, asrt *assert.Assertions) {
			asrt.Equal(*static.TypedSpec(), *r.TypedSpec())
			asrt.Equal([]string{"example.com", "example.org", "example.net"}, r.TypedSpec().SearchDomains)
		},
	)

	suite.Require().NoError(suite.state.Destroy(suite.ctx, static.Metadata()))

	suite.assertResolvers(
		[]string{
			"resolvers",
		}, func(r *network.ResolverSpec, asrt *assert.Assertions) {
			asrt.Equal([]netip.Addr{netip.MustParseAddr("1.1.2.0"), netip.MustParseAddr("1.1.2.1")}, r.TypedSpec().DNSServers)
		},
	)
}

func (suite *ResolverMergeSuite) TestMergeIPv46() {
	def := network.NewResolverSpec(network.ConfigNamespaceName, "default/resolvers")
	*def.TypedSpec() = network.ResolverSpecSpec{
		DNSServers: []netip.Addr{
			netip.MustParseAddr(constants.DefaultPrimaryResolver),
			netip.MustParseAddr(constants.DefaultSecondaryResolver),
		},
		ConfigLayer: network.ConfigDefault,
	}

	platform := network.NewResolverSpec(network.ConfigNamespaceName, "platform/resolvers")
	*platform.TypedSpec() = network.ResolverSpecSpec{
		DNSServers:  []netip.Addr{netip.MustParseAddr("1.1.2.0"), netip.MustParseAddr("fe80::1")},
		ConfigLayer: network.ConfigPlatform,
	}

	dhcp := network.NewResolverSpec(network.ConfigNamespaceName, "dhcp/eth1")
	*dhcp.TypedSpec() = network.ResolverSpecSpec{
		DNSServers:  []netip.Addr{netip.MustParseAddr("1.1.2.1")},
		ConfigLayer: network.ConfigOperator,
	}

	for _, res := range []resource.Resource{def, platform, dhcp} {
		suite.Require().NoError(suite.state.Create(suite.ctx, res), "%v", res.Spec())
	}

	suite.assertResolvers(
		[]string{
			"resolvers",
		}, func(r *network.ResolverSpec, asrt *assert.Assertions) {
			asrt.Equal(network.ConfigOperator, r.TypedSpec().ConfigLayer)
			asrt.Equal(`["1.1.2.1" "fe80::1"]`, fmt.Sprintf("%q", r.TypedSpec().DNSServers))
		},
	)
}

func (suite *ResolverMergeSuite) TearDownTest() {
	suite.T().Log("tear down")

	suite.ctxCancel()

	suite.wg.Wait()
}

func TestResolverMergeSuite(t *testing.T) {
	suite.Run(t, new(ResolverMergeSuite))
}
