// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package commands

import (
	"context"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	"github.com/siderolabs/talos/cmd/talosctl/pkg/talos/global"
	_ "github.com/siderolabs/talos/pkg/grpc/codec" // register codec
	"github.com/siderolabs/talos/pkg/machinery/client"
)

var kubernetesFlag bool

// GlobalArgs is the common arguments for the root command.
var GlobalArgs global.Args

var Config struct {
	RootDir       string
	GlobalOptions struct {
		Talosconfig string `yaml:"talosconfig"`
	} `yaml:"globalOptions"`
	TemplateOptions struct {
		Offline           bool     `yaml:"offline"`
		ValueFiles        []string `yaml:"valueFiles"`
		Values            []string `yaml:"values"`
		StringValues      []string `yaml:"stringValues"`
		FileValues        []string `yaml:"fileValues"`
		JsonValues        []string `yaml:"jsonValues"`
		LiteralValues     []string `yaml:"literalValues"`
		TalosVersion      string   `yaml:"talosVersion"`
		WithSecrets       string   `yaml:"withSecrets"`
		KubernetesVersion string   `yaml:"kubernetesVersion"`
		Full              bool     `yaml:"full"`
	} `yaml:"templateOptions"`
	ApplyOptions struct {
		DryRun           bool   `yaml:"preserve"`
		Timeout          string `yaml:"timeout"`
		TimeoutDuration  time.Duration
		CertFingerprints []string `yaml:"certFingerprints"`
	} `yaml:"applyOptions"`
	UpgradeOptions struct {
		Preserve bool `yaml:"preserve"`
		Stage    bool `yaml:"stage"`
		Force    bool `yaml:"force"`
	} `yaml:"upgradeOptions"`
}

const pathAutoCompleteLimit = 500

// WithClientNoNodes wraps common code to initialize Talos client and provide cancellable context.
//
// WithClientNoNodes doesn't set any node information on the request context.
func WithClientNoNodes(action func(context.Context, *client.Client) error, dialOptions ...grpc.DialOption) error {
	return GlobalArgs.WithClientNoNodes(action, dialOptions...)
}

// WithClient builds upon WithClientNoNodes to provide set of nodes on request context based on config & flags.
func WithClient(action func(context.Context, *client.Client) error, dialOptions ...grpc.DialOption) error {
	return GlobalArgs.WithClient(action, dialOptions...)
}

// WithClientMaintenance wraps common code to initialize Talos client in maintenance (insecure mode).
func WithClientMaintenance(enforceFingerprints []string, action func(context.Context, *client.Client) error) error {
	return GlobalArgs.WithClientMaintenance(enforceFingerprints, action)
}

// Commands is a list of commands published by the package.
var Commands []*cobra.Command

func addCommand(cmd *cobra.Command) {
	Commands = append(Commands, cmd)
}
