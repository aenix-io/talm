VERSION=$(shell git describe --tags)
TALOS_VERSION=$(shell  go list -m github.com/siderolabs/talos | awk '{sub(/^v/, "", $$NF); print $$NF}')

generate:
	go generate

build:
	go build -ldflags="-X 'main.Version=$(VERSION)'"

import: import-internal import-commands

import-commands:
	go run tools/import_commands.go --talos-version v$(TALOS_VERSION) \
    bootstrap \
    containers \
    dashboard \
    disks \
    dmesg \
    events \
    get \
    health \
    image \
    kubeconfig \
    list \
    logs \
    memory \
    mounts \
    netstat \
    pcap \
    processes \
    read \
    reboot \
    reset \
    restart \
    rollback \
    service \
    shutdown \
    stats \
    time \
    version

import-internal:
	rm -rf internal/pkg internal/app
	wget -O- https://github.com/siderolabs/talos/archive/refs/tags/v$(TALOS_VERSION).tar.gz | tar --strip=1 -xzf- \
		talos-$(TALOS_VERSION)/internal/app/apid \
		talos-$(TALOS_VERSION)/internal/app/dashboard \
		talos-$(TALOS_VERSION)/internal/app/machined \
		talos-$(TALOS_VERSION)/internal/app/maintenance \
		talos-$(TALOS_VERSION)/internal/app/poweroff \
		talos-$(TALOS_VERSION)/internal/app/resources \
		talos-$(TALOS_VERSION)/internal/app/storaged \
		talos-$(TALOS_VERSION)/internal/app/syslogd \
		talos-$(TALOS_VERSION)/internal/app/trustd \
		talos-$(TALOS_VERSION)/internal/app/wrapperd
		talos-$(TALOS_VERSION)/internal/pkg/capability \
		talos-$(TALOS_VERSION)/internal/pkg/cgroup \
		talos-$(TALOS_VERSION)/internal/pkg/configuration \
		talos-$(TALOS_VERSION)/internal/pkg/console \
		talos-$(TALOS_VERSION)/internal/pkg/containers \
		talos-$(TALOS_VERSION)/internal/pkg/cri \
		talos-$(TALOS_VERSION)/internal/pkg/ctxutil \
		talos-$(TALOS_VERSION)/internal/pkg/dashboard \
		talos-$(TALOS_VERSION)/internal/pkg/discovery \
		talos-$(TALOS_VERSION)/internal/pkg/dns \
		talos-$(TALOS_VERSION)/internal/pkg/encryption \
		talos-$(TALOS_VERSION)/internal/pkg/endpoint \
		talos-$(TALOS_VERSION)/internal/pkg/environment \
		talos-$(TALOS_VERSION)/internal/pkg/etcd \
		talos-$(TALOS_VERSION)/internal/pkg/extensions \
		talos-$(TALOS_VERSION)/internal/pkg/install \
		talos-$(TALOS_VERSION)/internal/pkg/logind \
		talos-$(TALOS_VERSION)/internal/pkg/meta \
		talos-$(TALOS_VERSION)/internal/pkg/miniprocfs \
		talos-$(TALOS_VERSION)/internal/pkg/mount \
		talos-$(TALOS_VERSION)/internal/pkg/ntp \
		talos-$(TALOS_VERSION)/internal/pkg/partition \
		talos-$(TALOS_VERSION)/internal/pkg/pcap \
		talos-$(TALOS_VERSION)/internal/pkg/pci \
		talos-$(TALOS_VERSION)/internal/pkg/secureboot \
		talos-$(TALOS_VERSION)/internal/pkg/smbios \
		talos-$(TALOS_VERSION)/internal/pkg/timex \
		talos-$(TALOS_VERSION)/internal/pkg/toml
	sed -i 's|github.com/siderolabs/talos/internal|github.com/aenix-io/talm/internal|g' `grep -rl 'github.com/siderolabs/talos/internal' internal`
