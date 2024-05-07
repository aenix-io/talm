// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	snapshot "go.etcd.io/etcd/etcdutl/v3/snapshot"

	"github.com/siderolabs/talos/pkg/logging"
	machineapi "github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
)

var bootstrapCmdFlags struct {
	configFiles          []string // -f/--files
	recoverFrom          string
	recoverSkipHashCheck bool
}

// bootstrapCmd represents the bootstrap command.
var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap",
	Short: "Bootstrap the etcd cluster on the specified node.",
	Long: `When Talos cluster is created etcd service on control plane nodes enter the join loop waiting
to join etcd peers from other control plane nodes. One node should be picked as the boostrap node.
When boostrap command is issued, the node aborts join process and bootstraps etcd cluster as a single node cluster.
Other control plane nodes will join etcd cluster once Kubernetes is boostrapped on the bootstrap node.

This command should not be used when "init" type node are used.

Talos etcd cluster can be recovered from a known snapshot with '--recover-from=' flag.`,
	Args: cobra.NoArgs,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if len(bootstrapCmdFlags.configFiles) > 1 {
			return fmt.Errorf("command \"bootstrap\" is not supported with multiple --file")
		}
		nodesFromArgs := len(GlobalArgs.Nodes) > 0
		endpointsFromArgs := len(GlobalArgs.Endpoints) > 0
		for _, configFile := range bootstrapCmdFlags.configFiles {
			if err := processModelineAndUpdateGlobals(configFile, nodesFromArgs, endpointsFromArgs, true); err != nil {
				return err
			}
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return WithClient(func(ctx context.Context, c *client.Client) error {
			if len(GlobalArgs.Nodes) > 1 {
				return fmt.Errorf("command \"bootstrap\" is not supported with multiple nodes")
			}

			if bootstrapCmdFlags.recoverFrom != "" {
				manager := snapshot.NewV3(logging.Wrap(os.Stderr))

				status, err := manager.Status(bootstrapCmdFlags.recoverFrom)
				if err != nil {
					return err
				}

				fmt.Printf("recovering from snapshot %q: hash %08x, revision %d, total keys %d, total size %d\n",
					bootstrapCmdFlags.recoverFrom, status.Hash, status.Revision, status.TotalKey, status.TotalSize)

				snapshot, err := os.Open(bootstrapCmdFlags.recoverFrom)
				if err != nil {
					return fmt.Errorf("error opening snapshot file: %w", err)
				}

				defer snapshot.Close() //nolint:errcheck

				_, err = c.EtcdRecover(ctx, snapshot)
				if err != nil {
					return fmt.Errorf("error uploading snapshot: %w", err)
				}
			}

			if err := c.Bootstrap(ctx, &machineapi.BootstrapRequest{
				RecoverEtcd:          bootstrapCmdFlags.recoverFrom != "",
				RecoverSkipHashCheck: bootstrapCmdFlags.recoverSkipHashCheck,
			}); err != nil {
				return fmt.Errorf("error executing bootstrap: %w", err)
			}

			return nil
		})
	},
}

func init() {
	bootstrapCmd.Flags().StringSliceVarP(&bootstrapCmdFlags.configFiles, "file", "f", nil, "specify config file or patch in a YAML file")
	bootstrapCmd.Flags().StringVar(&bootstrapCmdFlags.recoverFrom, "recover-from", "", "recover etcd cluster from the snapshot")
	bootstrapCmd.Flags().BoolVar(&bootstrapCmdFlags.recoverSkipHashCheck, "recover-skip-hash-check", false, "skip integrity check when recovering etcd (use when recovering from data directory copy)")
	addCommand(bootstrapCmd)
}
