// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"unsafe"

	"gopkg.in/yaml.v3"

	"github.com/cosi-project/runtime/pkg/resource"
	"github.com/cosi-project/runtime/pkg/resource/meta"
	"github.com/hashicorp/go-multierror"
	"github.com/spf13/cobra"

	"github.com/siderolabs/talos/cmd/talosctl/pkg/talos/helpers"
	"github.com/siderolabs/talos/pkg/machinery/client"
)

var getCmdFlags struct {
	insecure bool

	namespace string
	output    string
	watch     bool
}

// getCmd represents the get (resources) command.
var getCmd = &cobra.Command{
	Use:        "get <type> [<id>]",
	Aliases:    []string{"g"},
	SuggestFor: []string{},
	Short:      "Get a specific resource or list of resources (use 'talosctl get rd' to see all available resource types).",
	Long: `Similar to 'kubectl get', 'talosctl get' returns a set of resources from the OS.
To get a list of all available resource definitions, issue 'talosctl get rd'`,
	Example: "",
	Args:    cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if getCmdFlags.insecure {
			return WithClientMaintenance(nil, getResources(args))
		}

		return WithClient(getResources(args))
	},
}

//nolint:gocyclo,cyclop
func getResources(args []string) func(ctx context.Context, c *client.Client) error {
	return func(ctx context.Context, c *client.Client) error {

		var multiErr *multierror.Error

		var resources []Resource

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

		helperErr := helpers.ForEachResource(ctx, c, callbackRD, callbackResource, getCmdFlags.namespace, args...)
		if helperErr != nil {
			return helperErr
		}

		out, _ := json.Marshal(resources)
		fmt.Println(string(out))

		return multiErr.ErrorOrNil()
	}

}

func init() {
	getCmd.Flags().StringVar(&getCmdFlags.namespace, "namespace", "", "resource namespace (default is to use default namespace per resource)")
	getCmd.Flags().BoolVarP(&getCmdFlags.insecure, "insecure", "i", false, "get resources using the insecure (encrypted with no auth) maintenance service")
	addCommand(getCmd)
}

func readUnexportedField(field reflect.Value) any {
	return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface()
}

type Resource struct {
	Metadata any `json:"metadata"`
	Spec     any `json:"spec"`
}

func extractResourceData(r resource.Resource) (Resource, error) {
	// extract metadata
	o, _ := resource.MarshalYAML(r)
	m, _ := yaml.Marshal(o)
	var res Resource

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
			res.Spec = unmarshalledData
		} else {
			return res, fmt.Errorf("field 'yaml' not found")
		}
	}

	return res, nil
}
