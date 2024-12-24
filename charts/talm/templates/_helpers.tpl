{{- define "talm.discovered.system_disk_name" }}
{{- $disk := "" }}
{{- range .Disks }}
{{- if and (eq $disk "") .wwid }}
{{- $disk = .device_name }}
{{- end }}
{{- if .system_disk }}
{{- $disk = .device_name }}
{{- end }}
{{- end }}
{{- $disk }}
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
{{- if .wwid }}
# {{ .device_name }}:
#    model: {{ .model }}
#    serial: {{ .serial }}
#    wwid: {{ .wwid }}
#    size: {{ include "talm.human_size" .size }}
{{- end }}
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

{{- define "talm.discovered.default_addresses_by_gateway" }}
{{- $linkName := "" }}
{{- $family := "" }}
{{- range (lookup "routes" "" "").items }}
{{- if and (eq .spec.dst "") (not (eq .spec.gateway "")) (eq .spec.table "main") }}
{{- $linkName = .spec.outLinkName }}
{{- $family = .spec.family }}
{{- end }}
{{- end }}
{{- $addresses := list }}
{{- range (lookup "addresses" "" "").items }}
{{- if and (eq .spec.linkName $linkName) (eq .spec.family $family) (not (eq .spec.scope "host")) }}
{{- if not (hasPrefix (printf "%s/" $.Values.floatingIP) .spec.address) }}
{{- $addresses = append $addresses .spec.address }}
{{- end }}
{{- end }}
{{- end }}
{{- toJson $addresses }}
{{- end }}

{{- define "talm.discovered.physical_links_info" }}
# -- Discovered interfaces:
{{- range (lookup "links" "" "").items }}
{{- if and .spec.busPath (regexMatch "^(eno|eth|enp|enx|ens)" .metadata.id) }}
# enx{{ .spec.hardwareAddr | replace ":" "" }}:
#   id: {{ .metadata.id }}
#   hardwareAddr:{{ .spec.hardwareAddr }}
#   busPath: {{ .spec.busPath }}
#   driver: {{ .spec.driver }}
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

{{- define "talm.discovered.default_link_name_by_gateway" }}
{{- range (lookup "routes" "" "").items }}
{{- if and (eq .spec.dst "") (not (eq .spec.gateway "")) }}
{{- .spec.outLinkName }}
{{- end }}
{{- end }}
{{- end }}

{{- define "talm.discovered.default_link_address_by_gateway" }}
{{- range (lookup "routes" "" "").items }}
{{- if and (eq .spec.dst "") (not (eq .spec.gateway "")) }}
{{- (lookup "links" "" .spec.outLinkName).spec.hardwareAddr }}
{{- end }}
{{- end }}
{{- end }}

{{- define "talm.discovered.default_link_bus_by_gateway" }}
{{- range (lookup "routes" "" "").items }}
{{- if and (eq .spec.dst "") (not (eq .spec.gateway "")) }}
{{- (lookup "links" "" .spec.outLinkName).spec.hardwareAddr }}
{{- end }}
{{- end }}
{{- end }}

{{- define "talm.discovered.default_link_selector_by_gateway" }}
{{- range (lookup "routes" "" "").items }}
{{- if and (eq .spec.dst "") (not (eq .spec.gateway "")) }}
{{- with (lookup "links" "" .spec.outLinkName) }}
busPath: {{ .spec.busPath }}
{{- break }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}

{{- define "talm.predictable_link_name" -}}
{{ printf "enx%s" (lookup "links" "" . | dig "spec" "hardwareAddr" . | replace ":" "") }}
{{- end }}

{{- define "talm.discovered.default_gateway" }}
{{- range (lookup "routes" "" "").items }}
{{- if and (eq .spec.dst "") (not (eq .spec.gateway "")) (eq .spec.table "main") }}
{{- .spec.gateway }}
{{- break }}
{{- end }}
{{- end }}
{{- end }}

{{- define "talm.discovered.default_resolvers" }}
{{- with (lookup "resolvers" "" "resolvers") }}
{{- toJson .spec.dnsServers }}
{{- end }}
{{- end }}
