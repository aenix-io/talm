// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package commands

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/aenix-io/talm/pkg/engine"
	"github.com/siderolabs/gen/maps"
	"github.com/siderolabs/gen/xslices"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"

	"github.com/siderolabs/talos/cmd/talosctl/cmd/common"
	"github.com/siderolabs/talos/cmd/talosctl/pkg/talos/action"
	"github.com/siderolabs/talos/cmd/talosctl/pkg/talos/helpers"
	"github.com/siderolabs/talos/pkg/cli"
	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"github.com/siderolabs/talos/pkg/machinery/config/configloader"
)

var upgradeCmdFlags struct {
	trackableActionCmdFlags
	rebootMode        string
	preserve          bool
	stage             bool
	force             bool
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
}

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade Talos on the target node",
	Long:  ``,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if upgradeCmdFlags.debug {
			upgradeCmdFlags.wait = true
		}

		if upgradeCmdFlags.wait && upgradeCmdFlags.insecure {
			return fmt.Errorf("cannot use --wait and --insecure together")
		}

		if upgradeCmdFlags.insecure {
			return WithClientMaintenance(nil, upgrade(args))
		}

		return WithClient(upgrade(args))
	},
}

func upgrade(args []string) func(ctx context.Context, c *client.Client) error {
	return func(ctx context.Context, c *client.Client) error {
		rebootModeStr := strings.ToUpper(upgradeCmdFlags.rebootMode)

		rebootMode, rebootModeOk := machine.UpgradeRequest_RebootMode_value[rebootModeStr]
		if !rebootModeOk {
			return fmt.Errorf("invalid reboot mode: %s", upgradeCmdFlags.rebootMode)
		}

		// Gather image from config
		templateOpts := engine.Options{
			Insecure:          upgradeCmdFlags.insecure,
			ValueFiles:        upgradeCmdFlags.valueFiles,
			StringValues:      upgradeCmdFlags.stringValues,
			Values:            upgradeCmdFlags.values,
			FileValues:        upgradeCmdFlags.fileValues,
			JsonValues:        upgradeCmdFlags.jsonValues,
			LiteralValues:     upgradeCmdFlags.literalValues,
			TalosVersion:      upgradeCmdFlags.talosVersion,
			WithSecrets:       upgradeCmdFlags.withSecrets,
			Full:              true,
			Root:              Config.RootDir,
			Offline:           false,
			KubernetesVersion: upgradeCmdFlags.kubernetesVersion,
			TemplateFiles:     args,
		}
		result, err := engine.Render(ctx, c, templateOpts)
		if err != nil {
			return fmt.Errorf("failed to render templates: %w", err)
		}
		config, err := configloader.NewFromBytes(result)
		if err != nil {
			return err
		}

		image := config.Machine().Install().Image()
		if image == "" {
			return fmt.Errorf("error getting image from config")
		}

		opts := []client.UpgradeOption{
			client.WithUpgradeImage(image),
			client.WithUpgradeRebootMode(machine.UpgradeRequest_RebootMode(rebootMode)),
			client.WithUpgradePreserve(upgradeCmdFlags.preserve),
			client.WithUpgradeStage(upgradeCmdFlags.stage),
			client.WithUpgradeForce(upgradeCmdFlags.force),
		}

		if !upgradeCmdFlags.wait {
			return runUpgradeNoWait(opts)
		}

		common.SuppressErrors = true

		return action.NewTracker(
			&GlobalArgs,
			action.MachineReadyEventFn,
			func(ctx context.Context, c *client.Client) (string, error) {
				return upgradeGetActorID(ctx, c, opts)
			},
			action.WithPostCheck(action.BootIDChangedPostCheckFn),
			action.WithDebug(upgradeCmdFlags.debug),
			action.WithTimeout(upgradeCmdFlags.timeout),
		).Run()
	}
}

func runUpgradeNoWait(opts []client.UpgradeOption) error {
	upgradeFn := func(ctx context.Context, c *client.Client) error {
		if err := helpers.ClientVersionCheck(ctx, c); err != nil {
			return err
		}

		var remotePeer peer.Peer

		opts = append(opts, client.WithUpgradeGRPCCallOptions(grpc.Peer(&remotePeer)))

		// TODO: See if we can validate version and prevent starting upgrades to an unknown version
		resp, err := c.UpgradeWithOptions(ctx, opts...)
		if err != nil {
			if resp == nil {
				return fmt.Errorf("error performing upgrade: %s", err)
			}

			cli.Warning("%s", err)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "NODE\tACK\tSTARTED")

		defaultNode := client.AddrFromPeer(&remotePeer)

		for _, msg := range resp.Messages {
			node := defaultNode

			if msg.Metadata != nil {
				node = msg.Metadata.Hostname
			}

			fmt.Fprintf(w, "%s\t%s\t%s\t\n", node, msg.Ack, time.Now())
		}

		return w.Flush()
	}

	if upgradeCmdFlags.insecure {
		return WithClientMaintenance(nil, upgradeFn)
	}

	return WithClient(upgradeFn)
}

func upgradeGetActorID(ctx context.Context, c *client.Client, opts []client.UpgradeOption) (string, error) {
	resp, err := c.UpgradeWithOptions(ctx, opts...)
	if err != nil {
		return "", err
	}

	if len(resp.GetMessages()) == 0 {
		return "", fmt.Errorf("no messages returned from action run")
	}

	return resp.GetMessages()[0].GetActorId(), nil
}

func init() {
	rebootModes := maps.Keys(machine.UpgradeRequest_RebootMode_value)
	sort.Slice(rebootModes, func(i, j int) bool {
		return machine.UpgradeRequest_RebootMode_value[rebootModes[i]] < machine.UpgradeRequest_RebootMode_value[rebootModes[j]]
	})

	rebootModes = xslices.Map(rebootModes, strings.ToLower)

	upgradeCmd.Flags().StringVarP(&upgradeCmdFlags.rebootMode, "reboot-mode", "m", strings.ToLower(machine.UpgradeRequest_DEFAULT.String()),
		fmt.Sprintf("select the reboot mode during upgrade. Mode %q bypasses kexec. Valid values are: %q.",
			strings.ToLower(machine.UpgradeRequest_POWERCYCLE.String()),
			rebootModes))
	upgradeCmd.Flags().BoolVarP(&upgradeCmdFlags.preserve, "preserve", "p", Config.UpgradeOptions.Preserve, "preserve data")
	upgradeCmd.Flags().BoolVarP(&upgradeCmdFlags.stage, "stage", "", Config.UpgradeOptions.Stage, "stage the upgrade to perform it after a reboot")
	upgradeCmd.Flags().BoolVarP(&upgradeCmdFlags.force, "force", "", Config.UpgradeOptions.Force, "force the upgrade (skip checks on etcd health and members, might lead to data loss)")
	upgradeCmdFlags.addTrackActionFlags(upgradeCmd)

	upgradeCmd.Flags().BoolVarP(&upgradeCmdFlags.insecure, "insecure", "i", false, "apply using the insecure (encrypted with no auth) maintenance service")
	upgradeCmd.Flags().StringSliceVarP(&upgradeCmdFlags.valueFiles, "values", "f", Config.TemplateOptions.ValueFiles, "specify values in a YAML file (can specify multiple)")
	upgradeCmd.Flags().StringArrayVar(&upgradeCmdFlags.values, "set", Config.TemplateOptions.Values, "set values on the command line (can specify multiple or separate values with commas: key1=val1,key2=val2)")
	upgradeCmd.Flags().StringArrayVar(&upgradeCmdFlags.stringValues, "set-string", Config.TemplateOptions.StringValues, "set STRING values on the command line (can specify multiple or separate values with commas: key1=val1,key2=val2)")
	upgradeCmd.Flags().StringArrayVar(&upgradeCmdFlags.fileValues, "set-file", Config.TemplateOptions.FileValues, "set values from respective files specified via the command line (can specify multiple or separate values with commas: key1=path1,key2=path2)")
	upgradeCmd.Flags().StringArrayVar(&upgradeCmdFlags.jsonValues, "set-json", Config.TemplateOptions.JsonValues, "set JSON values on the command line (can specify multiple or separate values with commas: key1=jsonval1,key2=jsonval2)")
	upgradeCmd.Flags().StringArrayVar(&upgradeCmdFlags.literalValues, "set-literal", Config.TemplateOptions.LiteralValues, "set a literal STRING value on the command line")
	upgradeCmd.Flags().StringVar(&upgradeCmdFlags.talosVersion, "talos-version", Config.TemplateOptions.TalosVersion, "the desired Talos version to generate config for (backwards compatibility, e.g. v0.8)")
	upgradeCmd.Flags().StringVar(&upgradeCmdFlags.withSecrets, "with-secrets", Config.TemplateOptions.WithSecrets, "use a secrets file generated using 'gen secrets'")
	upgradeCmd.Flags().StringVar(&upgradeCmdFlags.kubernetesVersion, "kubernetes-version", Config.TemplateOptions.KubernetesVersion, "desired kubernetes version to run")
	addCommand(upgradeCmd)
}
