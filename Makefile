VENDOR := web/vendor
BIN    := recui

# DM Mono Regular (latin subset, weight 400) from Google Fonts static CDN.
# To update: visit https://fonts.google.com/specimen/DM+Mono, inspect the
# @font-face CSS for the latin subset, and copy the woff2 src URL.
DM_MONO_URL := https://fonts.gstatic.com/s/dmmono/v16/aFTU7PB1QTsUX8KYthqQBA.woff2

.PHONY: build vendor clean cleanbuild help

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-12s %s\n", $$1, $$2}'

build: vendor ## Build the recui binary
	go build -o $(BIN) ./cmd/recui/

vendor: $(VENDOR)/dm-mono.woff2 ## Download the vendored DM Mono font (skips if present)

$(VENDOR)/dm-mono.woff2:
	mkdir -p $(VENDOR)
	curl -fsSL "$(DM_MONO_URL)" -o $@

clean: ## Delete vendored files and binary
	rm -rf $(VENDOR) $(BIN)

cleanbuild: clean build ## Clean then build from scratch
