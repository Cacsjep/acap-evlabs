## Voicer build entry points.
#
# Targets that need cross-compilation (build / dev / prod) shell out to
# goxisbuilder, which runs the Axis SDK Docker image and produces the .eap.
# Host targets (host / host-test / web) work without Docker so the Go logic
# and Vue UI can be iterated on Windows / Linux / macOS.

IP ?= 10.0.0.48
PWD ?= 1qay2wsx
SDK ?= 12.5.0
APPDIR := ./ax_voicer
WEBDIR := ./web

# Single-binary build: cgo links libpipewire-0.3 directly into the Go binary
# inside the goxisbuilder Docker image. No separate helper, no -files needed
# beyond the static frontend.
GOXIS_FLAGS := -appdir $(APPDIR) -ip $(IP) -pwd $(PWD) -sdk $(SDK) \
               -files "html" \
               -ignore ".git web .venv build node_modules" \
               -upx=false -nocopy

## ------------------ device ------------------

dev: web-build ## install + watch on $(IP)
	goxisbuilder $(GOXIS_FLAGS) -install -watch -tags "dev"

build: web-build ## build .eap and install on $(IP)
	goxisbuilder $(GOXIS_FLAGS) -install -tags "dev"

prod: web-build ## build .eap (no install)
	goxisbuilder $(GOXIS_FLAGS) -tags "prod"

## ------------------ host ------------------

host: ## run the backend on this host (no goxis, mock pipewire)
	go run -tags=host,mock ./ax_voicer -listen :8889 -html ./ax_voicer/html

host-test: ## go test ./... with host+mock build tags
	go test -tags=host,mock ./...

host-vet:
	go vet -tags=host,mock ./...

web: ## run vite dev server (proxies to camera at $(IP))
	npm --prefix $(WEBDIR) run dev

web-build: ## production frontend → ax_voicer/html
	npm --prefix $(WEBDIR) run build

web-install: ## npm install for the frontend
	npm --prefix $(WEBDIR) install


## ------------------ misc ------------------

tidy:
	go mod tidy

clean:
	rm -rf $(APPDIR)/html
	rm -f  $(APPDIR)/voicer
	rm -rf $(APPDIR)/build

help:
	@grep -E '^[a-zA-Z_-]+:.*?##' Makefile | sort | awk -F':.*?## ' '{printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

.PHONY: dev build prod host host-test host-vet web web-build web-install tidy clean help
