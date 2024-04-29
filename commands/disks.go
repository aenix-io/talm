// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package commands

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/siderolabs/talos/pkg/cli"
	"github.com/siderolabs/talos/pkg/machinery/client"
)

var disksCmdFlags struct {
	insecure bool
}

var disksCmd = &cobra.Command{
	Use:   "disks",
	Short: "Get the list of disks from /sys/block on the machine",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		if disksCmdFlags.insecure {
			return WithClientMaintenance(nil, printDisks)
		}

		return WithClient(printDisks)
	},
}

//nolint:gocyclo
func printDisks(ctx context.Context, c *client.Client) error {
	response, err := c.Disks(ctx)
	if err != nil {
		if response == nil {
			return fmt.Errorf("error getting disks: %w", err)
		}

		cli.Warning("%s", err)
	}
	out, _ := json.Marshal(response.Messages)
	fmt.Println(string(out))

	return nil
}

func init() {
	disksCmd.Flags().BoolVarP(&disksCmdFlags.insecure, "insecure", "i", false, "get disks using the insecure (encrypted with no auth) maintenance service")
	addCommand(disksCmd)
}
