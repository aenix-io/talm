// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"unsafe"

	"gopkg.in/yaml.v3"

	"github.com/aenix-io/talm/pkg/engine"
	"github.com/aenix-io/talm/pkg/yamltools"
	"github.com/cosi-project/runtime/pkg/resource"
	"github.com/cosi-project/runtime/pkg/resource/meta"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/strvals"

	"github.com/siderolabs/talos/cmd/talosctl/pkg/talos/helpers"

	"github.com/siderolabs/talos/pkg/cli"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"github.com/siderolabs/talos/pkg/machinery/config"
	"github.com/siderolabs/talos/pkg/machinery/config/bundle"
	"github.com/siderolabs/talos/pkg/machinery/config/configpatcher"
	"github.com/siderolabs/talos/pkg/machinery/config/encoder"
	"github.com/siderolabs/talos/pkg/machinery/config/generate"
	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"github.com/siderolabs/talos/pkg/machinery/config/machine"
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
			return WithClientMaintenance(nil, render(args))
		}

		return WithClient(render(args))
	},
}

func render(args []string) func(ctx context.Context, c *client.Client) error {
	return func(ctx context.Context, c *client.Client) error {
		if !templateCmdFlags.offline {
			if err := helpers.FailIfMultiNodes(ctx, "talm template"); err != nil {
				return err
			}

			response, err := c.Disks(ctx)
			if err != nil {
				if response == nil {
					return fmt.Errorf("error getting disks: %w", err)
				}

				cli.Warning("%s", err)
			}
			for _, m := range response.Messages {
				for _, d := range m.Disks {
					dj, err := json.Marshal(d)
					if err != nil {
						return err
					}
					var disk map[string]interface{}
					err = json.Unmarshal(dj, &disk)
					if err != nil {
						return err
					}
					engine.Disks[d.DeviceName] = disk
				}
			}

			engine.LookupFunc = newLookupFunction(ctx, c)
		}

		chartPath, err := os.Getwd()
		if err != nil {
			return err
		}
		if templateCmdFlags.root != "" {
			chartPath = templateCmdFlags.root
		}

		// Load chart
		chrt, err := loader.LoadDir(chartPath)
		if err != nil {
			return err
		}

		// Load user values
		var values map[string]interface{}
		for _, filePath := range templateCmdFlags.valueFiles {
			vals, err := chartutil.ReadValuesFile(filePath)
			if err != nil {
				return err
			}
			values = chartutil.CoalesceTables(vals, chrt.Values)
		}

		// Load cmd values
		vals, err := MergeValues()
		if err != nil {
			return err
		}
		values = chartutil.CoalesceTables(vals, values)

		rootValues := map[string]interface{}{
			"Values": values,
		}

		// Render chart
		eng := engine.Engine{}
		out, err := eng.Render(chrt, rootValues)
		if err != nil {
			return err
		}

		var configPatches []string
		for _, arg := range args {
			requestedTemplate := filepath.Join(chrt.Name(), arg)
			configPatch, ok := out[requestedTemplate]
			if !ok {
				return fmt.Errorf("template %s not found", arg)
			}
			configPatches = append(configPatches, configPatch)
		}

		var genOptions []generate.Option //nolint:prealloc

		if templateCmdFlags.talosVersion != "" {
			var versionContract *config.VersionContract

			versionContract, err = config.ParseContractFromVersion(templateCmdFlags.talosVersion)
			if err != nil {
				return fmt.Errorf("invalid talos-version: %w", err)
			}

			genOptions = append(genOptions, generate.WithVersionContract(versionContract))
		}

		if templateCmdFlags.withSecrets != "" {
			var secretsBundle *secrets.Bundle

			secretsBundle, err = secrets.LoadBundle(templateCmdFlags.withSecrets)
			if err != nil {
				return fmt.Errorf("failed to load secrets bundle: %w", err)
			}

			genOptions = append(genOptions, generate.WithSecretsBundle(secretsBundle))
		}

		configFinal := []byte(configPatches[len(configPatches)-1])

		configBundleOpts := []bundle.Option{
			bundle.WithInputOptions(
				&bundle.InputOptions{
					KubeVersion: strings.TrimPrefix(templateCmdFlags.kubernetesVersion, "v"),
					GenOptions:  genOptions,
				},
			),
			bundle.WithVerbose(false),
		}

		// Load patches
		patches, err := configpatcher.LoadPatches(configPatches)
		if err != nil {
			return err
		}

		// Load config and apply patches to it to discover machineType
		configBundle, err := bundle.NewBundle(configBundleOpts...)
		if err != nil {
			return err
		}
		err = configBundle.ApplyPatches(patches, true, true)
		if err != nil {
			return err
		}
		machineType := configBundle.ControlPlaneCfg.Machine().Type()
		if machineType == machine.TypeUnknown {
			machineType = machine.TypeWorker
		}

		// Reload config with correct machineType and apply patches to it
		configBundle, err = bundle.NewBundle(configBundleOpts...)
		if err != nil {
			return err
		}
		var configOrigin []byte
		if !templateCmdFlags.full {
			configOrigin, err = configBundle.Serialize(encoder.CommentsDisabled, machineType)
			if err != nil {
				return err
			}

			// Overwrite machine.type to preserve this field for diff
			var config map[string]interface{}
			if err := yaml.Unmarshal(configOrigin, &config); err != nil {
				return err
			}
			if machine, ok := config["machine"].(map[string]interface{}); ok {
				machine["type"] = "unknown"
			}
			configOrigin, err = yaml.Marshal(&config)
			if err != nil {
				return err
			}
		}
		err = configBundle.ApplyPatches(patches, true, true)
		if err != nil {
			return err
		}

		// Render config
		configFull, err := configBundle.Serialize(encoder.CommentsDisabled, machineType)
		if err != nil {
			return err
		}
		configPatches = append(configPatches, string(configFull))

		// Calculate diff patch
		var target []byte
		if templateCmdFlags.full {
			target = []byte(configPatches[len(configPatches)-1])
		} else {
			target, err = yamltools.DiffYAMLs(configOrigin, configFull)
			if err != nil {
				return nil
			}
		}

		// Copy comments
		var targetNode yaml.Node
		if err := yaml.Unmarshal(target, &targetNode); err != nil {
			return err
		}
		for _, configPatch := range configPatches[:len(configPatches)-1] {
			var sourceNode yaml.Node
			if err := yaml.Unmarshal([]byte(configPatch), &sourceNode); err != nil {
				return err
			}

			dstPaths := make(map[string]*yaml.Node)
			yamltools.CopyComments(&sourceNode, &targetNode, "", dstPaths)
			yamltools.ApplyComments(&targetNode, "", dstPaths)
		}
		configFinal, err = yaml.Marshal(&targetNode)
		if err != nil {
			return err
		}

		fmt.Println(string(configFinal))

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

func newLookupFunction(ctx context.Context, c *client.Client) func(resource string, namespace string, id string) (map[string]interface{}, error) {
	return func(kind string, namespace string, id string) (map[string]interface{}, error) {
		var multiErr *multierror.Error

		var resources []map[string]interface{}

		// get <type>
		// get <type> <id>
		callbackResource := func(parentCtx context.Context, hostname string, r resource.Resource, callError error) error {
			if callError != nil {
				multiErr = multierror.Append(multiErr, callError)
				return nil
			}

			res, err := extractResourceData(r)
			if err != nil {
				return nil
			}

			resources = append(resources, res)

			return nil
		}
		callbackRD := func(definition *meta.ResourceDefinition) error {
			return nil
		}

		helperErr := helpers.ForEachResource(ctx, c, callbackRD, callbackResource, namespace, kind, id)
		if helperErr != nil {
			return map[string]interface{}{}, helperErr
		}
		if len(resources) == 0 {
			return map[string]interface{}{}, nil
		}
		if id != "" && len(resources) == 1 {
			return resources[0], nil
		}
		items := map[string]interface{}{}
		for i, res := range resources {
			items["_"+strconv.Itoa(i)] = res
		}
		return map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "List",
			"items":      items,
		}, nil
	}
}

func readUnexportedField(field reflect.Value) any {
	return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface()
}

// builds resource with metadata, spec and stringSpec fields
func extractResourceData(r resource.Resource) (map[string]interface{}, error) {
	// extract metadata
	o, _ := resource.MarshalYAML(r)
	m, _ := yaml.Marshal(o)
	var res map[string]interface{}

	yaml.Unmarshal(m, &res)

	// extract spec
	val := reflect.ValueOf(r.Spec())
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() == reflect.Struct {
		if yamlField := val.FieldByName("yaml"); yamlField.IsValid() {
			yamlValue := readUnexportedField(yamlField)
			var unmarshalledData any
			if err := yaml.Unmarshal([]byte(yamlValue.(string)), &unmarshalledData); err != nil {
				return res, fmt.Errorf("error unmarshaling yaml: %w", err)
			}
			res["spec"] = unmarshalledData
			//res["stringSpec"] = yamlValue.(string)
		} else {
			return res, fmt.Errorf("field 'yaml' not found")
		}
	}

	return res, nil
}

// Imported from Helm
// https://github.com/helm/helm/blob/c6beb169d26751efd8131a5d65abe75c81a334fb/pkg/cli/values/options.go#L44
func MergeValues() (map[string]interface{}, error) {
	opts := templateCmdFlags
	base := map[string]interface{}{}

	// User specified a values files via -f/--values
	for _, filePath := range opts.valueFiles {
		currentMap := map[string]interface{}{}

		bytes, err := os.ReadFile(filePath)
		if err != nil {
			return nil, err
		}

		if err := yaml.Unmarshal(bytes, &currentMap); err != nil {
			return nil, errors.Wrapf(err, "failed to parse %s", filePath)
		}
		// Merge with the previous map
		base = mergeMaps(base, currentMap)
	}

	// User specified a value via --set-json
	for _, value := range opts.jsonValues {
		if err := strvals.ParseJSON(value, base); err != nil {
			return nil, errors.Errorf("failed parsing --set-json data %s", value)
		}
	}

	// User specified a value via --set
	for _, value := range opts.values {
		if err := strvals.ParseInto(value, base); err != nil {
			return nil, errors.Wrap(err, "failed parsing --set data")
		}
	}

	// User specified a value via --set-string
	for _, value := range opts.stringValues {
		if err := strvals.ParseIntoString(value, base); err != nil {
			return nil, errors.Wrap(err, "failed parsing --set-string data")
		}
	}

	// User specified a value via --set-file
	for _, value := range opts.fileValues {
		reader := func(rs []rune) (interface{}, error) {
			bytes, err := os.ReadFile(value)
			if err != nil {
				return nil, err
			}
			return string(bytes), err
		}
		if err := strvals.ParseIntoFile(value, base, reader); err != nil {
			return nil, errors.Wrap(err, "failed parsing --set-file data")
		}
	}

	// User specified a value via --set-literal
	for _, value := range opts.literalValues {
		if err := strvals.ParseLiteralInto(value, base); err != nil {
			return nil, errors.Wrap(err, "failed parsing --set-literal data")
		}
	}

	return base, nil
}

// Imported from Helm
// https://github.com/helm/helm/blob/c6beb169d26751efd8131a5d65abe75c81a334fb/pkg/cli/values/options.go#L108
func mergeMaps(a, b map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(a))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		if v, ok := v.(map[string]interface{}); ok {
			if bv, ok := out[k]; ok {
				if bv, ok := bv.(map[string]interface{}); ok {
					out[k] = mergeMaps(bv, v)
					continue
				}
			}
		}
		out[k] = v
	}
	return out
}
