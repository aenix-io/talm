// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/siderolabs/talos/cmd/talosctl/cmd/mgmt/gen"
	"github.com/siderolabs/talos/pkg/machinery/config"
	"github.com/siderolabs/talos/pkg/machinery/config/generate"
	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
)

var chartFileContent = `name: %s
version: 0.1.0
globalOptions:
  talosconfig: "talosconfig"
templateOptions:
  offline: false
  valueFiles: []
  values: []
  stringValues: []
  fileValues: []
  jsonValues: []
  literalValues: []
  talosVersion: ""
  withSecrets: "secrets.yaml"
  kubernetesVersion: ""
  full: false
applyOptions:
  preserve: false
  timeout: "1m"
  certFingerprints: []
upgradeOptions:
  preserve: false
  stage: false
  force: false
`

var helpersFileContent = `{{- define "talos.discovered_system_disk_name" }}
{{- range .Disks }}
{{- if .system_disk }}
{{- .device_name }}
{{- end }}
{{- end }}
{{- end }}

{{- define "talos.discovered_machinetype" }}
{{- (lookup "machinetype" "" "machine-type").spec }}
{{- end }}

{{- define "talos.discovered_hostname" }}
{{- with (lookup "hostname" "" "hostname") }}
{{- .spec.hostname }}
{{- end }}
{{- end }}

{{- define "talos.discovered_disks_info" }}
# -- Discovered disks:
{{- range .Disks }}
# {{ .device_name }}:
#    model: {{ .model }}
#    serial: {{ .serial }}
#    wwid: {{ .wwid }}
#    size: {{ include "talos.human_size" .size }}
{{- end }}
{{- end }}

{{- define "talos.human_size" }}
  {{- $bytes := int64 . }}
  {{- if lt $bytes 1048576 }}
    {{- printf "%.2f MB" (divf $bytes 1048576.0) }}
  {{- else if lt $bytes 1073741824 }}
    {{- printf "%.2f GB" (divf $bytes 1073741824.0) }}
  {{- else }}
    {{- printf "%.2f TB" (divf $bytes 1099511627776.0) }}
  {{- end }}
{{- end }}

{{- define "talos.discovered_default_addresses" }}
{{- with (lookup "nodeaddress" "" "default") }}
{{- toJson .spec.addresses }}
{{- end }}
{{- end }}


{{- define "talos.discovered_physical_links_info" }}
# -- Discovered interfaces:
{{- range (lookup "links" "" "").items }}
{{- if regexMatch "^(eno|eth|enp|enx|ens)" .metadata.id }}
# enx{{ .spec.permanentAddr | replace ":" "" }}:
#   name: {{ .metadata.id }}
#   mac:{{ .spec.hardwareAddr }}
#   bus:{{ .spec.busPath }}
#   driver:{{ .spec.driver }}
#   vendor: {{ .spec.vendor }}
#   product: {{ .spec.product }})
{{- end }}
{{- end }}
{{- end }}

{{- define "talos.discovered_default_link_name" }}
{{- range (lookup "addresses" "" "").items }}
{{- if has .spec.address (fromJsonArray (include "talos.discovered_default_addresses" .)) }}
{{- .spec.linkName }}
{{- end }}
{{- end }}
{{- end }}

{{- define "talos.predictable_link_name" -}}
enx{{ lookup "links" "" . | dig "spec" "permanentAddr" . | replace ":" "" }}
{{- end }}

{{- define "talos.discovered_default_gateway" }}
{{- range (lookup "routes" "" "").items }}
{{- if and (eq .spec.dst "") (not (eq .spec.gateway "")) }}
{{- .spec.gateway }}
{{- end }}
{{- end }}
{{- end }}

{{- define "talos.discovered_default_resolvers" }}
{{- with (lookup "resolvers" "" "resolvers") }}
{{- toJson .spec.dnsServers }}
{{- end }}
{{- end }}
`

var controlPlaneFileContent = `machine:
  type: controlplane
  install:
    {{- (include "talos.discovered_disks_info" .) | nindent 4 }}

    disk: {{ include "talos.discovered_system_disk_name" . | quote }}
  network:
    hostname: {{ include "talos.discovered_hostname" . | quote }}
    nameservers: {{ include "talos.discovered_default_resolvers" . }}
    {{- (include "talos.discovered_physical_links_info" .) | nindent 4 }}

    interfaces:
    {{- $defaultLink := (include "talos.discovered_default_link_name" .) }}
    - interface: {{ include "talos.predictable_link_name" $defaultLink }}
      addresses: {{ include "talos.discovered_default_addresses" . }}
      routes:
        - network: 0.0.0.0/0
          gateway: {{ include "talos.discovered_default_gateway" . }}

cluster:
  clusterName: "{{ .Chart.Name }}"
  controlPlane:
    endpoint: "https://192.168.0.1:6443"
`

var workerFileContent = `machine:
  type: worker
  install:
    {{- (include "talos.discovered_disks_info" .) | nindent 4 }}

    disk: {{ include "talos.discovered_system_disk_name" . | quote }}
  network:
    hostname: {{ include "talos.discovered_hostname" . | quote }}
    nameservers: {{ include "talos.discovered_default_resolvers" . }}
    {{- (include "talos.discovered_physical_links_info" .) | nindent 4 }}

    interfaces:
    {{- $defaultLink := (include "talos.discovered_default_link_name" .) }}
    - interface: {{ include "talos.predictable_link_name" $defaultLink }}
      addresses: {{ include "talos.discovered_default_addresses" . }}
      routes:
        - network: 0.0.0.0/0
          gateway: {{ include "talos.discovered_default_gateway" . }}

cluster:
  clusterName: "{{ .Chart.Name }}"
  controlPlane:
    endpoint: "https://192.168.0.1:6443"
`

var initCmdFlags struct {
	force        bool
	talosVersion string
}

// initCmd represents the `init` command.
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new project and generate default values",
	Long:  ``,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		var (
			secretsBundle   *secrets.Bundle
			versionContract *config.VersionContract
			err             error
		)

		if initCmdFlags.talosVersion != "" {
			versionContract, err = config.ParseContractFromVersion(initCmdFlags.talosVersion)
			if err != nil {
				return fmt.Errorf("invalid talos-version: %w", err)
			}
		}

		secretsBundle, err = secrets.NewBundle(secrets.NewFixedClock(time.Now()),
			versionContract,
		)
		if err != nil {
			return fmt.Errorf("failed to create secrets bundle: %w", err)
		}
		var genOptions []generate.Option //nolint:prealloc
		if initCmdFlags.talosVersion != "" {
			var versionContract *config.VersionContract

			versionContract, err = config.ParseContractFromVersion(initCmdFlags.talosVersion)
			if err != nil {
				return fmt.Errorf("invalid talos-version: %w", err)
			}

			genOptions = append(genOptions, generate.WithVersionContract(versionContract))
		}
		genOptions = append(genOptions, generate.WithSecretsBundle(secretsBundle))

		err = writeSecretsBundleToFile(secretsBundle)
		if err != nil {
			return err
		}

		// Clalculate cluster name from directory
		absolutePath, err := filepath.Abs(Config.RootDir)
		if err != nil {
			return err
		}
		clusterName := filepath.Base(absolutePath)

		configBundle, err := gen.GenerateConfigBundle(genOptions, clusterName, "https://192.168.0.1:6443", "", []string{}, []string{}, []string{})
		configBundle.TalosConfig().Contexts[clusterName].Endpoints = []string{"127.0.0.1"}
		if err != nil {
			return err
		}

		data, err := yaml.Marshal(configBundle.TalosConfig())
		if err != nil {
			return fmt.Errorf("failed to marshal config: %+v", err)
		}

		talosconfigFile := filepath.Join(Config.RootDir, "talosconfig")
		if err = writeToDestination(data, talosconfigFile, 0o644); err != nil {
			return err
		}

		chartFile := filepath.Join(Config.RootDir, "Chart.yaml")
		if err = writeToDestination([]byte(fmt.Sprintf(chartFileContent, clusterName)), chartFile, 0o644); err != nil {
			return err
		}

		helpersFile := filepath.Join(Config.RootDir, "templates/_helpers.tpl")
		if err = writeToDestination([]byte(helpersFileContent), helpersFile, 0o644); err != nil {
			return err
		}

		controlPlaneFile := filepath.Join(Config.RootDir, "templates/controlplane.yaml")
		if err = writeToDestination([]byte(controlPlaneFileContent), controlPlaneFile, 0o644); err != nil {
			return err
		}
		workerFile := filepath.Join(Config.RootDir, "templates/worker.yaml")
		if err = writeToDestination([]byte(workerFileContent), workerFile, 0o644); err != nil {
			return err
		}

		return nil

	},
}

func writeSecretsBundleToFile(bundle *secrets.Bundle) error {
	bundleBytes, err := yaml.Marshal(bundle)
	if err != nil {
		return err
	}

	secretsFile := filepath.Join(Config.RootDir, "secrets.yaml")
	if err = validateFileExists(secretsFile); err != nil {
		return err
	}

	return writeToDestination(bundleBytes, secretsFile, 0o644)
}

func init() {
	initCmd.Flags().StringVar(&initCmdFlags.talosVersion, "talos-version", "", "the desired Talos version to generate config for (backwards compatibility, e.g. v0.8)")
	initCmd.Flags().BoolVar(&initCmdFlags.force, "force", false, "will overwrite existing files")

	initCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if !cmd.Flags().Changed("talos-version") {
			initCmdFlags.talosVersion = Config.TemplateOptions.TalosVersion
		}
		return nil
	}
	addCommand(initCmd)
}

func validateFileExists(file string) error {
	if !initCmdFlags.force {
		if _, err := os.Stat(file); err == nil {
			return fmt.Errorf("file %q already exists, use --force to overwrite", file)
		}
	}

	return nil
}

func writeToDestination(data []byte, destination string, permissions os.FileMode) error {
	if err := validateFileExists(destination); err != nil {
		return err
	}

	parentDir := filepath.Dir(destination)

	// Create dir path, ignoring "already exists" messages
	if err := os.MkdirAll(parentDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create output dir: %w", err)
	}

	err := os.WriteFile(destination, data, permissions)

	fmt.Fprintf(os.Stderr, "Created %s\n", destination)

	return err
}
