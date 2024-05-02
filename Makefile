TALOS_VERSION=$(shell	go list -m github.com/siderolabs/talos | awk '{sub(/^v/, "", $$NF); print $$NF}')
update-tui:
	rm -rf internal/pkg/tui
	wget -O- https://github.com/siderolabs/talos/archive/refs/tags/v$(TALOS_VERSION).tar.gz | tar --strip=1 -xzf- talos-$(TALOS_VERSION)/internal/pkg/tui
	sed -i 's|github.com/siderolabs/talos/internal|github.com/aenix-io/talm/internal|g' `grep -rl 'github.com/siderolabs/talos/internal' internal/pkg/tui`
