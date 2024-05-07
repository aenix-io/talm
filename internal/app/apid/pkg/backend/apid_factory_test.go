// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package backend_test

import (
	"crypto/tls"
	"sync"
	"testing"

	"github.com/siderolabs/grpc-proxy/proxy"
	"github.com/stretchr/testify/suite"

	"github.com/aenix-io/talm/internal/app/apid/pkg/backend"
)

type APIDFactorySuite struct {
	suite.Suite

	f *backend.APIDFactory
}

type fakeTLSConfigProvider struct{}

func (fakeTLSConfigProvider) ClientConfig() (*tls.Config, error) {
	return &tls.Config{}, nil
}

func (suite *APIDFactorySuite) SetupSuite() {
	suite.f = backend.NewAPIDFactory(fakeTLSConfigProvider{})
}

func (suite *APIDFactorySuite) TestGet() {
	b1, err := suite.f.Get("127.0.0.1")
	suite.Require().NoError(err)
	suite.Require().NotNil(b1)

	b2, err := suite.f.Get("127.0.0.1")
	suite.Require().NoError(err)
	suite.Require().Equal(b1, b2)

	b3, err := suite.f.Get("127.0.0.2")
	suite.Require().NoError(err)
	suite.Require().NotEqual(b1, b3)

	_, err = suite.f.Get("127.0.0.2:50000")
	suite.Require().Error(err)
}

func (suite *APIDFactorySuite) TestGetConcurrent() {
	// for race detector
	var wg sync.WaitGroup

	backendCh := make(chan proxy.Backend, 10)

	for range 10 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			b, _ := suite.f.Get("10.0.0.1") //nolint:errcheck
			backendCh <- b
		}()
	}

	wg.Wait()
	close(backendCh)

	b := <-backendCh

	for anotherB := range backendCh {
		suite.Assert().Equal(b, anotherB)
	}
}

func TestAPIDFactorySuite(t *testing.T) {
	suite.Run(t, new(APIDFactorySuite))
}
