{{- define "talm.discovered.system_disk_name" }}
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

{{- define "talm.discovered.default_link_name" }}
{{- range (lookup "addresses" "" "").items }}
{{- if has .spec.address (fromJsonArray (include "talm.discovered.default_addresses" .)) }}
{{- .spec.linkName }}
{{- end }}
{{- end }}
{{- end }}

{{- define "talm.predictable_link_name" -}}
enx{{ lookup "links" "" . | dig "spec" "permanentAddr" . | replace ":" "" }}
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
