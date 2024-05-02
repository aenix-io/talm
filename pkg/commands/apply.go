// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/aenix-io/talm/pkg/engine"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/aenix-io/talm/internal/pkg/tui/installer"
	"github.com/siderolabs/talos/cmd/talosctl/pkg/talos/helpers"
	machineapi "github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
)

var applyCmdFlags struct {
	helpers.Mode
	certFingerprints  []string
	insecure          bool
	valueFiles        []string // -f/--values
	stringValues      []string // --set-string
	values            []string // --set
	fileValues        []string // --set-file
	jsonValues        []string // --set-json
	literalValues     []string // --set-literal
	talosVersion      string
	withSecrets       string
	kubernetesVersion string
	dryRun            bool
	preserve          bool
	stage             bool
	force             bool
	configTryTimeout  time.Duration
}

var applyCmd = &cobra.Command{
	Use:   "apply <file ..>",
	Short: "Apply config to a Talos node",
	Long:  ``,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if applyCmdFlags.insecure {
			return WithClientMaintenance(nil, apply(args))
		}

		return WithClient(apply(args))
	},
}

func apply(args []string) func(ctx context.Context, c *client.Client) error {
	return func(ctx context.Context, c *client.Client) error {
		opts := engine.Options{
			Insecure:          applyCmdFlags.insecure,
			ValueFiles:        applyCmdFlags.valueFiles,
			StringValues:      applyCmdFlags.stringValues,
			Values:            applyCmdFlags.values,
			FileValues:        applyCmdFlags.fileValues,
			JsonValues:        applyCmdFlags.jsonValues,
			LiteralValues:     applyCmdFlags.literalValues,
			TalosVersion:      applyCmdFlags.talosVersion,
			WithSecrets:       applyCmdFlags.withSecrets,
			Full:              true,
			Root:              Config.RootDir,
			Offline:           false,
			KubernetesVersion: applyCmdFlags.kubernetesVersion,
			TemplateFiles:     args,
		}

		result, err := engine.Render(ctx, c, opts)
		if err != nil {
			return fmt.Errorf("failed to render templates: %w", err)
		}

		withClient := func(f func(context.Context, *client.Client) error) error {
			if applyCmdFlags.insecure {
				return WithClientMaintenance(applyCmdFlags.certFingerprints, f)
			}

			return WithClient(f)
		}

		return withClient(func(ctx context.Context, c *client.Client) error {
			if applyCmdFlags.Mode.Mode == helpers.InteractiveMode {
				install := installer.NewInstaller()
				node := GlobalArgs.Nodes[0]

				if len(GlobalArgs.Endpoints) > 0 {
					return WithClientNoNodes(func(bootstrapCtx context.Context, bootstrapClient *client.Client) error {
						opts := []installer.Option{}
						opts = append(opts, installer.WithBootstrapNode(bootstrapCtx, bootstrapClient, GlobalArgs.Endpoints[0]), installer.WithDryRun(applyCmdFlags.dryRun))

						conn, err := installer.NewConnection(
							ctx,
							c,
							node,
							opts...,
						)
						if err != nil {
							return err
						}

						return install.Run(conn)
					})
				}

				conn, err := installer.NewConnection(
					ctx,
					c,
					node,
					installer.WithDryRun(applyCmdFlags.dryRun),
				)
				if err != nil {
					return err
				}

				return install.Run(conn)
			}

			resp, err := c.ApplyConfiguration(ctx, &machineapi.ApplyConfigurationRequest{
				Data:           result,
				Mode:           applyCmdFlags.Mode.Mode,
				DryRun:         applyCmdFlags.dryRun,
				TryModeTimeout: durationpb.New(applyCmdFlags.configTryTimeout),
			})
			if err != nil {
				return fmt.Errorf("error applying new configuration: %s", err)
			}

			helpers.PrintApplyResults(resp)

			return nil
		})
	}

}

func init() {
	applyCmd.Flags().BoolVarP(&applyCmdFlags.insecure, "insecure", "i", false, "apply using the insecure (encrypted with no auth) maintenance service")
	applyCmd.Flags().StringSliceVarP(&applyCmdFlags.valueFiles, "values", "f", nil, "specify values in a YAML file (can specify multiple)")
	applyCmd.Flags().StringArrayVar(&applyCmdFlags.values, "set", nil, "set values on the command line (can specify multiple or separate values with commas: key1=val1,key2=val2)")
	applyCmd.Flags().StringArrayVar(&applyCmdFlags.stringValues, "set-string", nil, "set STRING values on the command line (can specify multiple or separate values with commas: key1=val1,key2=val2)")
	applyCmd.Flags().StringArrayVar(&applyCmdFlags.fileValues, "set-file", nil, "set values from respective files specified via the command line (can specify multiple or separate values with commas: key1=path1,key2=path2)")
	applyCmd.Flags().StringArrayVar(&applyCmdFlags.jsonValues, "set-json", nil, "set JSON values on the command line (can specify multiple or separate values with commas: key1=jsonval1,key2=jsonval2)")
	applyCmd.Flags().StringArrayVar(&applyCmdFlags.literalValues, "set-literal", nil, "set a literal STRING value on the command line")
	applyCmd.Flags().StringVar(&applyCmdFlags.talosVersion, "talos-version", "", "the desired Talos version to generate config for (backwards compatibility, e.g. v0.8)")
	applyCmd.Flags().StringVar(&applyCmdFlags.withSecrets, "with-secrets", "", "use a secrets file generated using 'gen secrets'")

	applyCmd.Flags().StringVar(&applyCmdFlags.kubernetesVersion, "kubernetes-version", "", "desired kubernetes version to run")

	applyCmd.Flags().BoolVar(&applyCmdFlags.dryRun, "dry-run", false, "check how the config change will be applied in dry-run mode")
	applyCmd.Flags().DurationVar(&applyCmdFlags.configTryTimeout, "timeout", 0, "the config will be rolled back after specified timeout (if try mode is selected)")
	applyCmd.Flags().StringSliceVar(&applyCmdFlags.certFingerprints, "cert-fingerprint", nil, "list of server certificate fingeprints to accept (defaults to no check)")
	helpers.AddModeFlags(&applyCmdFlags.Mode, applyCmd)

	applyCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		applyCmdFlags.valueFiles = append(Config.TemplateOptions.ValueFiles, applyCmdFlags.valueFiles...)
		applyCmdFlags.values = append(Config.TemplateOptions.Values, applyCmdFlags.values...)
		applyCmdFlags.stringValues = append(Config.TemplateOptions.StringValues, applyCmdFlags.stringValues...)
		applyCmdFlags.fileValues = append(Config.TemplateOptions.FileValues, applyCmdFlags.fileValues...)
		applyCmdFlags.jsonValues = append(Config.TemplateOptions.JsonValues, applyCmdFlags.jsonValues...)
		applyCmdFlags.literalValues = append(Config.TemplateOptions.LiteralValues, applyCmdFlags.literalValues...)
		if !cmd.Flags().Changed("talos-version") {
			applyCmdFlags.talosVersion = Config.TemplateOptions.TalosVersion
		}
		if !cmd.Flags().Changed("with-secrets") {
			applyCmdFlags.withSecrets = Config.TemplateOptions.WithSecrets
		}
		if !cmd.Flags().Changed("kubernetes-version") {
			applyCmdFlags.kubernetesVersion = Config.TemplateOptions.KubernetesVersion
		}
		if !cmd.Flags().Changed("preserve") {
			applyCmdFlags.preserve = Config.UpgradeOptions.Preserve
		}
		if !cmd.Flags().Changed("stage") {
			applyCmdFlags.stage = Config.UpgradeOptions.Stage
		}
		if !cmd.Flags().Changed("force") {
			applyCmdFlags.force = Config.UpgradeOptions.Force
		}
		return nil
	}

	addCommand(applyCmd)
}
