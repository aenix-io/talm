// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strconv"
	"unsafe"

	"gopkg.in/yaml.v3"

	"github.com/aenix-io/talm/pkg/engine"
	"github.com/cosi-project/runtime/pkg/resource"
	"github.com/cosi-project/runtime/pkg/resource/meta"
	"github.com/hashicorp/go-multierror"
	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"

	"github.com/siderolabs/talos/cmd/talosctl/pkg/talos/helpers"
	"github.com/siderolabs/talos/pkg/cli"
	"github.com/siderolabs/talos/pkg/machinery/client"
)

var templateCmdFlags struct {
	insecure bool
	values   []string
}

var templateCmd = &cobra.Command{
	Use:   "template",
	Short: "Render chart templates locally and display the output",
	Long:  ``,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if templateCmdFlags.insecure {
			return WithClientMaintenance(nil, render(args))
		}

		return WithClient(render(args))
	},
}

func render(args []string) func(ctx context.Context, c *client.Client) error {
	return func(ctx context.Context, c *client.Client) error {
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

		chartPath := args[0]

		// Load chart
		chrt, err := loader.LoadDir(chartPath)
		if err != nil {
			return err
		}
		values, err := chartutil.ReadValuesFile(filepath.Join(chartPath, "values.yaml"))
		if err != nil {
			return err
		}

		// Load user values
		for _, filePath := range templateCmdFlags.values {
			v, err := chartutil.ReadValuesFile(filePath)
			if err != nil {
				return err
			}
			values = chartutil.CoalesceTables(v, values)
		}

		rootValues := map[string]interface{}{
			"Values": values,
		}

		// Render chart
		eng := engine.Engine{}
		out, err := eng.Render(chrt, rootValues)
		if err != nil {
			return err
		}

		// Output
		for _, v := range out {
			//fmt.Printf("---\n# Source: %s\n%s\n", k, v)
			fmt.Printf("%s\n", v)
		}

		return nil
	}
}

func init() {
	templateCmd.Flags().StringSliceVarP(&templateCmdFlags.values, "values", "f", nil, "specify values in a YAML file (can specify multiple)")
	templateCmd.Flags().BoolVarP(&templateCmdFlags.insecure, "insecure", "i", false, "template using the insecure (encrypted with no auth) maintenance service")
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
