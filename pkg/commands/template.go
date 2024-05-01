// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package commands

import (
	"bytes"
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
	"github.com/siderolabs/talos/pkg/machinery/config/bundle"
	"github.com/siderolabs/talos/pkg/machinery/config/configpatcher"
	"github.com/siderolabs/talos/pkg/machinery/config/encoder"
	"github.com/siderolabs/talos/pkg/machinery/config/generate"
	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"github.com/siderolabs/talos/pkg/machinery/config/machine"
)

var templateCmdFlags struct {
	insecure      bool
	valueFiles    []string // -f/--values
	stringValues  []string // --set-string
	values        []string // --set
	fileValues    []string // --set-file
	jsonValues    []string // --set-json
	literalValues []string // --set-literal
	withSecrets   string
	full          bool
	root          string
	offline       bool
}

var templateCmd = &cobra.Command{
	Use:   "template",
	Short: "Render chart templates locally and display the output",
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

		if templateCmdFlags.withSecrets != "" {
			var secretsBundle *secrets.Bundle

			secretsBundle, err = secrets.LoadBundle(templateCmdFlags.withSecrets)
			if err != nil {
				return fmt.Errorf("failed to load secrets bundle: %w", err)
			}

			genOptions = append(genOptions, generate.WithSecretsBundle(secretsBundle))
		}

		if err != nil {
			return err
		}

		configFinal := []byte(configPatches[len(configPatches)-1])
		configBundleOpts := []bundle.Option{
			bundle.WithInputOptions(
				&bundle.InputOptions{
					ClusterName: "clusterName",
					Endpoint:    "endpoint",
					KubeVersion: strings.TrimPrefix("kubernetesVersion", "v"),
					GenOptions:  genOptions,
				},
			),
		}
		patches, err := configpatcher.LoadPatches(configPatches)
		configBundle, err := bundle.NewBundle(configBundleOpts...)
		var configOrigin []byte
		if !templateCmdFlags.full {
			configOrigin, err = configBundle.Serialize(encoder.CommentsDisabled, machine.TypeControlPlane)
			if err != nil {
				return err
			}
		}
		configBundle.ApplyPatches(patches, true, true)
		configFull, err := configBundle.Serialize(encoder.CommentsDisabled, machine.TypeControlPlane)
		if err != nil {
			return err
		}
		configPatches = append(configPatches, string(configFull))
		// Copy comments
		if len(configPatches) > 1 {

			var target []byte
			if templateCmdFlags.full {
				target = []byte(configPatches[len(configPatches)-1])
			} else {
				target, err = diffYAMLs(configOrigin, configFull)
				if err != nil {
					return nil
				}
			}
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
				copyComments(&sourceNode, &targetNode, "", dstPaths)
				applyComments(&targetNode, "", dstPaths)
			}
			configFinal, err = yaml.Marshal(&targetNode)
			if err != nil {
				return err
			}
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
	templateCmd.Flags().StringVar(&templateCmdFlags.withSecrets, "with-secrets", "", "use a secrets file generated using 'gen secrets'")
	templateCmd.Flags().BoolVarP(&templateCmdFlags.full, "full", "", false, "show full resulting config, not only patch")
	templateCmd.Flags().StringVar(&templateCmdFlags.root, "root", "", "root directory of the project")
	templateCmd.Flags().BoolVarP(&templateCmdFlags.offline, "offline", "", false, "disable gathering information and lookup functions")

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

// copyComments updates the comments in dstNode considering the structure of whitespace.
func copyComments(srcNode, dstNode *yaml.Node, path string, dstPaths map[string]*yaml.Node) {
	// Save the path of the current node in dstPaths if there are comments.
	if srcNode.HeadComment != "" || srcNode.LineComment != "" || srcNode.FootComment != "" {
		dstPaths[path] = srcNode
	}

	// Recursive traversal for child elements.
	for i := 0; i < len(srcNode.Content); i++ {
		newPath := path + "/" + srcNode.Content[i].Value // For nodes with keys.
		if srcNode.Kind == yaml.SequenceNode {
			newPath = path + "/" + string(i) // For lists.
		}
		copyComments(srcNode.Content[i], dstNode, newPath, dstPaths)
	}
}

// applyComments applies the copied comments to the target document.
func applyComments(dstNode *yaml.Node, path string, dstPaths map[string]*yaml.Node) {
	if srcNode, ok := dstPaths[path]; ok {
		dstNode.HeadComment = mergeComments(dstNode.HeadComment, srcNode.HeadComment)
		dstNode.LineComment = mergeComments(dstNode.LineComment, srcNode.LineComment)
		dstNode.FootComment = mergeComments(dstNode.FootComment, srcNode.FootComment)
	}

	// Apply to child elements.
	for i := 0; i < len(dstNode.Content); i++ {
		newPath := path + "/" + dstNode.Content[i].Value // For nodes with keys.
		if dstNode.Kind == yaml.SequenceNode {
			newPath = path + "/" + string(i) // For lists.
		}
		applyComments(dstNode.Content[i], newPath, dstPaths)
	}
}

// mergeComments combines old and new comments considering empty lines.
func mergeComments(oldComment, newComment string) string {
	if oldComment == "" {
		return newComment
	}
	if newComment == "" {
		return oldComment
	}
	return strings.TrimSpace(oldComment) + "\n\n" + strings.TrimSpace(newComment)
}

// ----------------
// diffYAMLs compares two YAML documents and outputs the differences, including relevant comments.
// TODO: comments are not compared, and they should not
// TODO: lists should contain only missing items
func diffYAMLs(original, modified []byte) ([]byte, error) {
	var origNode, modNode yaml.Node
	if err := yaml.Unmarshal(original, &origNode); err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(modified, &modNode); err != nil {
		return nil, err
	}

	diff := compareNodes(origNode.Content[0], modNode.Content[0])
	if diff == nil { // If there are no differences
		return []byte{}, nil
	}

	buffer := &bytes.Buffer{}
	encoder := yaml.NewEncoder(buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(diff); err != nil {
		return nil, err
	}
	encoder.Close()

	return buffer.Bytes(), nil
}

// compareNodes recursively finds differences between two YAML nodes.
func compareNodes(orig, mod *yaml.Node) *yaml.Node {
	if orig.Kind != mod.Kind {
		return mod // Different kinds means definitely changed.
	}

	switch orig.Kind {
	case yaml.MappingNode:
		return compareMappingNodes(orig, mod)
	case yaml.SequenceNode:
		return compareSequenceNodes(orig, mod)
	case yaml.ScalarNode:
		if orig.Value != mod.Value {
			return mod // Different scalar values mean changed.
		}
	}
	return nil // No differences found
}

// compareMappingNodes compares two mapping nodes and returns differences.
func compareMappingNodes(orig, mod *yaml.Node) *yaml.Node {
	diff := &yaml.Node{Kind: yaml.MappingNode}
	origMap := nodeMap(orig)
	modMap := nodeMap(mod)

	for k, modVal := range modMap {
		origVal, ok := origMap[k]
		if !ok {
			// Key not in original, it's an addition.
			addNodeToDiff(diff, k, modVal)
		} else {
			changedNode := compareNodes(origVal, modVal)
			if changedNode != nil {
				addNodeToDiff(diff, k, changedNode)
			}
		}
	}

	if len(diff.Content) == 0 {
		return nil // No differences.
	}
	return diff
}

// compareSequenceNodes compares two sequence nodes.
func compareSequenceNodes(orig, mod *yaml.Node) *yaml.Node {
	// Simple sequence comparison: by index (naive but effective for many use cases).
	diff := &yaml.Node{Kind: yaml.SequenceNode}
	minLength := min(len(orig.Content), len(mod.Content))
	for i := 0; i < minLength; i++ {
		changedNode := compareNodes(orig.Content[i], mod.Content[i])
		if changedNode != nil {
			diff.Content = append(diff.Content, changedNode)
		}
	}
	if len(mod.Content) > minLength { // Additional items in mod
		diff.Content = append(diff.Content, mod.Content[minLength:]...)
	}

	if len(diff.Content) == 0 {
		return nil
	}
	return diff
}

// Utility functions

// nodeMap creates a map from a YAML mapping node for easy lookup.
func nodeMap(node *yaml.Node) map[string]*yaml.Node {
	result := make(map[string]*yaml.Node)
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		if keyNode.Kind == yaml.ScalarNode {
			result[keyNode.Value] = node.Content[i+1]
		}
	}
	return result
}

// addNodeToDiff adds a node to the diff result.
func addNodeToDiff(diff *yaml.Node, key string, node *yaml.Node) {
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
	diff.Content = append(diff.Content, keyNode)
	diff.Content = append(diff.Content, node)
}

// min returns the smaller of x or y.
func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}
