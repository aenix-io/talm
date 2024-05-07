VERSION=$(shell git describe --tags)
TALOS_VERSION=$(shell  go list -m github.com/siderolabs/talos | awk '{sub(/^v/, "", $$NF); print $$NF}')

generate: update-dashboard
	go generate

build:
	go build -ldflags="-X 'main.Version=$(VERSION)'"

update-dashboard:
	rm -rf internal/pkg/dashboard internal/pkg/meta internal/app/machined/pkg/runtime
	wget -O- https://github.com/siderolabs/talos/archive/refs/tags/v$(TALOS_VERSION).tar.gz | tar --strip=1 -xzf- talos-$(TALOS_VERSION)/internal/pkg/dashboard talos-$(TALOS_VERSION)/internal/pkg/meta talos-$(TALOS_VERSION)/internal/app/machined/pkg/runtime
	sed -i 's|github.com/siderolabs/talos/internal|github.com/aenix-io/talm/internal|g' `grep -rl 'github.com/siderolabs/talos/internal' internal/pkg/dashboard internal/pkg/meta internal/app/machined/pkg/runtime`
