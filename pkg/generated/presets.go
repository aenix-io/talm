package generated

var PresetFiles = map[string]string{
	"cozystack/Chart.yaml": `apiVersion: v2
name: %s
type: application
version: %s
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
  talosVersion: "v1.6"
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
`,
	"cozystack/templates/_helpers.tpl": `{{- define "talos.config" }}
machine:
  type: {{ .MachineType }}
  kubelet:
    nodeIP:
      validSubnets:
        {{- toYaml .Values.advertisedSubnets | nindent 8 }}
    extraConfig:
      maxPods: 512
  kernel:
    modules:
    - name: openvswitch
    - name: drbd
      parameters:
        - usermode_helper=disabled
    - name: zfs
    - name: spl
  files:
  - content: |
      [plugins]
        [plugins."io.containerd.grpc.v1.cri"]
          device_ownership_from_security_context = true      
    path: /etc/cri/conf.d/20-customization.part
    op: create
  install:
    {{- with .Values.image }}
    image: {{ . }}
    {{- end }}
    {{- (include "talm.discovered.disks_info" .) | nindent 4 }}
    disk: {{ include "talm.discovered.system_disk_name" . | quote }}
  network:
    hostname: {{ include "talm.discovered.hostname" . | quote }}
    nameservers: {{ include "talm.discovered.default_resolvers" . }}
    {{- (include "talm.discovered.physical_links_info" .) | nindent 4 }}
    interfaces:
    {{- $defaultLink := (include "talm.discovered.default_link_name" .) }}
    - interface: {{ include "talm.predictable_link_name" $defaultLink }}
      addresses: {{ include "talm.discovered.default_addresses" . }}
      routes:
        - network: 0.0.0.0/0
          gateway: {{ include "talm.discovered.default_gateway" . }}
      {{- with .Values.floatingIP }}
      vip:
        ip: {{ . }}
      {{- end }}

cluster:
  network:
    cni:
      name: none
    podSubnets:
      {{- toYaml .Values.podSubnets | nindent 6 }}
    serviceSubnets:
      {{- toYaml .Values.serviceSubnets | nindent 6 }}
  clusterName: "{{ .Chart.Name }}"
  controlPlane:
    endpoint: "{{ .Values.endpoint }}"
  {{- if eq .MachineType "controlplane" }}
  allowSchedulingOnControlPlanes: true
  controllerManager:
    extraArgs:
      bind-address: 0.0.0.0
  scheduler:
    extraArgs:
      bind-address: 0.0.0.0
  apiServer:
    certSANs:
    - 127.0.0.1
  proxy:
    disabled: true
  discovery:
    enabled: false
  etcd:
    advertisedSubnets:
      {{- toYaml .Values.advertisedSubnets | nindent 6 }}
  {{- end }}
{{- end }}
`,
	"cozystack/templates/controlplane.yaml": `{{- $_ := set . "MachineType" "controlplane" -}}
{{- include "talos.config" . }}
`,
	"cozystack/templates/worker.yaml": `{{- $_ := set . "MachineType" "worker" -}}
{{- include "talos.config" . }}
`,
	"cozystack/values.yaml": `endpoint: "https://192.168.100.10:6443"
floatingIP: 192.168.100.10
image: "ghcr.io/aenix-io/cozystack/talos:v1.6.4"
podSubnets:
- 10.244.0.0/16
serviceSubnets:
- 10.96.0.0/16
advertisedSubnets:
- 192.168.100.0/24
`,
	"generic/Chart.yaml": `apiVersion: v2
name: %s
type: application
version: %s
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
`,
	"generic/templates/_helpers.tpl": `{{- define "talos.config" }}
machine:
  type: {{ .MachineType }}
  kubelet:
    nodeIP:
      validSubnets:
        {{- toYaml .Values.advertisedSubnets | nindent 8 }}
  install:
    {{- (include "talm.discovered.disks_info" .) | nindent 4 }}
    disk: {{ include "talm.discovered.system_disk_name" . | quote }}
  network:
    hostname: {{ include "talm.discovered.hostname" . | quote }}
    nameservers: {{ include "talm.discovered.default_resolvers" . }}
    {{- (include "talm.discovered.physical_links_info" .) | nindent 4 }}
    interfaces:
    {{- $defaultLink := (include "talm.discovered.default_link_name" .) }}
    - interface: {{ include "talm.predictable_link_name" $defaultLink }}
      addresses: {{ include "talm.discovered.default_addresses" . }}
      routes:
        - network: 0.0.0.0/0
          gateway: {{ include "talm.discovered.default_gateway" . }}
      {{- with .Values.floatingIP }}
      vip:
        ip: {{ . }}
      {{- end }}

cluster:
  network:
    podSubnets:
      {{- toYaml .Values.podSubnets | nindent 6 }}
    serviceSubnets:
      {{- toYaml .Values.serviceSubnets | nindent 6 }}
  clusterName: "{{ .Chart.Name }}"
  controlPlane:
    endpoint: "{{ .Values.endpoint }}"
  {{- if eq .MachineType "controlplane" }}
  etcd:
    advertisedSubnets:
      {{- toYaml .Values.advertisedSubnets | nindent 6 }}
  {{- end }}
{{- end }}
`,
	"generic/templates/controlplane.yaml": `{{- $_ := set . "MachineType" "controlplane" -}}
{{- include "talos.config" . }}
`,
	"generic/templates/worker.yaml": `{{- $_ := set . "MachineType" "worker" -}}
{{- include "talos.config" . }}
`,
	"generic/values.yaml": `endpoint: "https://192.168.100.10:6443"
podSubnets:
- 10.244.0.0/16
serviceSubnets:
- 10.96.0.0/16
advertisedSubnets:
- 192.168.100.0/24
`,
	"talm/Chart.yaml": `apiVersion: v2
type: library
name: %s
version: %s
description: A library Talm chart for Talos Linux
`,
	"talm/templates/_helpers.tpl": `{{- define "talm.discovered.system_disk_name" }}
{{- range .Disks }}
{{- if .system_disk }}
{{- .device_name }}
{{- end }}
{{- end }}
{{- end }}

{{- define "talm.discovered.machinetype" }}
{{- (lookup "machinetype" "" "machine-type").spec }}
{{- end }}

{{- define "talm.discovered.hostname" }}
{{- with (lookup "hostname" "" "hostname") }}
{{- .spec.hostname }}
{{- end }}
{{- end }}

{{- define "talm.discovered.disks_info" }}
# -- Discovered disks:
{{- range .Disks }}
# {{ .device_name }}:
#    model: {{ .model }}
#    serial: {{ .serial }}
#    wwid: {{ .wwid }}
#    size: {{ include "talm.human_size" .size }}
{{- end }}
{{- end }}

{{- define "talm.human_size" }}
  {{- $bytes := int64 . }}
  {{- if lt $bytes 1048576 }}
    {{- printf "%.2f MB" (divf $bytes 1048576.0) }}
  {{- else if lt $bytes 1073741824 }}
    {{- printf "%.2f GB" (divf $bytes 1073741824.0) }}
  {{- else }}
    {{- printf "%.2f TB" (divf $bytes 1099511627776.0) }}
  {{- end }}
{{- end }}

{{- define "talm.discovered.default_addresses" }}
{{- with (lookup "nodeaddress" "" "default") }}
{{- toJson .spec.addresses }}
{{- end }}
{{- end }}


{{- define "talm.discovered.physical_links_info" }}
# -- Discovered interfaces:
{{- range (lookup "links" "" "").items }}
{{- if regexMatch "^(eno|eth|enp|enx|ens)" .metadata.id }}
# enx{{ .spec.hardwareAddr | replace ":" "" }}:
#   name: {{ .metadata.id }}
#   mac:{{ .spec.hardwareAddr }}
#   bus:{{ .spec.busPath }}
#   driver:{{ .spec.driver }}
#   vendor: {{ .spec.vendor }}
#   product: {{ .spec.product }})
{{- end }}
{{- end }}
{{- end }}

{{- define "talm.discovered.default_link_name" }}
{{- range (lookup "addresses" "" "").items }}
{{- if has .spec.address (fromJsonArray (include "talm.discovered.default_addresses" .)) }}
{{- .spec.linkName }}
{{- end }}
{{- end }}
{{- end }}

{{- define "talm.predictable_link_name" -}}
{{ printf "enx%s" (lookup "links" "" . | dig "spec" "hardwareAddr" . | replace ":" "") }}
{{- end }}

{{- define "talm.discovered.default_gateway" }}
{{- range (lookup "routes" "" "").items }}
{{- if and (eq .spec.dst "") (not (eq .spec.gateway "")) }}
{{- .spec.gateway }}
{{- end }}
{{- end }}
{{- end }}

{{- define "talm.discovered.default_resolvers" }}
{{- with (lookup "resolvers" "" "resolvers") }}
{{- toJson .spec.dnsServers }}
{{- end }}
{{- end }}
`,
}

var AvailablePresets = []string{
	"generic",
	"cozystack",
}
