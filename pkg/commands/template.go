// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package commands

import (
	"context"
	"fmt"

	"github.com/aenix-io/talm/pkg/engine"
	"github.com/spf13/cobra"

	"github.com/siderolabs/talos/pkg/machinery/client"
	"github.com/siderolabs/talos/pkg/machinery/constants"
)

var templateCmdFlags struct {
	insecure          bool
	valueFiles        []string // -f/--values
	stringValues      []string // --set-string
	values            []string // --set
	fileValues        []string // --set-file
	jsonValues        []string // --set-json
	literalValues     []string // --set-literal
	talosVersion      string
	withSecrets       string
	full              bool
	root              string
	offline           bool
	kubernetesVersion string
}

var templateCmd = &cobra.Command{
	Use:   "template <file ..>",
	Short: "Render templates locally and display the output",
	Long:  ``,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if templateCmdFlags.insecure {
			return WithClientMaintenance(nil, template(args))
		}

		return WithClient(template(args))
	},
}

func template(args []string) func(ctx context.Context, c *client.Client) error {
	return func(ctx context.Context, c *client.Client) error {
		opts := engine.Options{
			// Populate the options based on the templateCmdFlags or any other necessary input
			Insecure:          templateCmdFlags.insecure,
			ValueFiles:        templateCmdFlags.valueFiles,
			StringValues:      templateCmdFlags.stringValues,
			Values:            templateCmdFlags.values,
			FileValues:        templateCmdFlags.fileValues,
			JsonValues:        templateCmdFlags.jsonValues,
			LiteralValues:     templateCmdFlags.literalValues,
			TalosVersion:      templateCmdFlags.talosVersion,
			WithSecrets:       templateCmdFlags.withSecrets,
			Full:              templateCmdFlags.full,
			Root:              templateCmdFlags.root,
			Offline:           templateCmdFlags.offline,
			KubernetesVersion: templateCmdFlags.kubernetesVersion,
			TemplateFiles:     args, // This is where your 'args' are used to specify the template files to render
		}

		// Call the Render function from the render package
		result, err := engine.Render(ctx, c, opts)
		if err != nil {
			return fmt.Errorf("failed to render templates: %w", err)
		}

		// Print the result to the standard output
		fmt.Println(string(result))

		return nil
	}
}

func init() {
	templateCmd.Flags().BoolVarP(&templateCmdFlags.insecure, "insecure", "i", false, "template using the insecure (encrypted with no auth) maintenance service")
	templateCmd.Flags().StringSliceVarP(&templateCmdFlags.valueFiles, "values", "f", []string{}, "specify values in a YAML file (can specify multiple)")
	templateCmd.Flags().StringArrayVar(&templateCmdFlags.values, "set", []string{}, "set values on the command line (can specify multiple or separate values with commas: key1=val1,key2=val2)")
	templateCmd.Flags().StringArrayVar(&templateCmdFlags.stringValues, "set-string", []string{}, "set STRING values on the command line (can specify multiple or separate values with commas: key1=val1,key2=val2)")
	templateCmd.Flags().StringArrayVar(&templateCmdFlags.fileValues, "set-file", []string{}, "set values from respective files specified via the command line (can specify multiple or separate values with commas: key1=path1,key2=path2)")
	templateCmd.Flags().StringArrayVar(&templateCmdFlags.jsonValues, "set-json", []string{}, "set JSON values on the command line (can specify multiple or separate values with commas: key1=jsonval1,key2=jsonval2)")
	templateCmd.Flags().StringArrayVar(&templateCmdFlags.literalValues, "set-literal", []string{}, "set a literal STRING value on the command line")
	templateCmd.Flags().StringVar(&templateCmdFlags.talosVersion, "talos-version", "", "the desired Talos version to generate config for (backwards compatibility, e.g. v0.8)")
	templateCmd.Flags().StringVar(&templateCmdFlags.withSecrets, "with-secrets", "", "use a secrets file generated using 'gen secrets'")
	templateCmd.Flags().BoolVarP(&templateCmdFlags.full, "full", "", false, "show full resulting config, not only patch")
	templateCmd.Flags().StringVar(&templateCmdFlags.root, "root", "", "root directory of the project")
	templateCmd.Flags().BoolVarP(&templateCmdFlags.offline, "offline", "", false, "disable gathering information and lookup functions")
	templateCmd.Flags().StringVar(&templateCmdFlags.kubernetesVersion, "kubernetes-version", constants.DefaultKubernetesVersion, "desired kubernetes version to run")

	addCommand(templateCmd)
}
