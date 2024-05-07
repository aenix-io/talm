// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package commands

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/aenix-io/talm/internal/pkg/dashboard"
	"github.com/siderolabs/talos/pkg/machinery/client"
)

var dashboardCmdFlags struct {
	configFiles []string // -f/--files
	interval    time.Duration
}

// dashboardCmd represents the monitor command.
var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Cluster dashboard with node overview, logs and real-time metrics",
	Long: `Provide a text-based UI to navigate node overview, logs and real-time metrics.

Keyboard shortcuts:

 - h, <Left>: switch one node to the left
 - l, <Right>: switch one node to the right
 - j, <Down>: scroll logs/process list down
 - k, <Up>: scroll logs/process list up
 - <C-d>: scroll logs/process list half page down
 - <C-u>: scroll logs/process list half page up
 - <C-f>: scroll logs/process list one page down
 - <C-b>: scroll logs/process list one page up
`,
	Args: cobra.NoArgs,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		nodesFromArgs := len(GlobalArgs.Nodes) > 0
		endpointsFromArgs := len(GlobalArgs.Endpoints) > 0
		for _, configFile := range dashboardCmdFlags.configFiles {
			if err := processModelineAndUpdateGlobals(configFile, nodesFromArgs, endpointsFromArgs, false); err != nil {
				return err
			}
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return WithClient(func(ctx context.Context, c *client.Client) error {
			return dashboard.Run(ctx, c,
				dashboard.WithInterval(dashboardCmdFlags.interval),
				dashboard.WithScreens(dashboard.ScreenSummary, dashboard.ScreenMonitor),
				dashboard.WithAllowExitKeys(true),
			)
		})
	},
}

func init() {
	dashboardCmd.Flags().StringSliceVarP(&dashboardCmdFlags.configFiles, "file", "f", nil, "specify config files or patches in a YAML file (can specify multiple)")
	dashboardCmd.Flags().DurationVarP(&dashboardCmdFlags.interval, "update-interval", "d", 3*time.Second, "interval between updates")
	addCommand(dashboardCmd)
}
