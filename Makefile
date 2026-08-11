PKG_PREFIX := vraxel.io/vraxel
APP_NAME   := vraxel-server

# Stamped into lib/buildinfo.Version; "dev" outside a git tree.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X '$(PKG_PREFIX)/lib/buildinfo.Version=$(APP_NAME)-$(VERSION)'

# The gitignored per-developer overlay wins when present; otherwise the
# committed config, whose defaults boot against localhost PG with
# authentication on.
CONFIG := $(if $(wildcard app/$(APP_NAME)/config.dev.yaml),app/$(APP_NAME)/config.dev.yaml,app/$(APP_NAME)/config.yaml)

.DEFAULT_GOAL := help
.PHONY: help dev build generate check fmt clean new-migration setup-hooks

help: ## List available commands
	@awk 'BEGIN{FS=":.*## "} /^[a-z][a-z-]*:.*## /{printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

dev: ## Run vraxel-server (:8088) + vite (:5173) with HMR
	@trap 'kill 0' EXIT; \
	go run $(PKG_PREFIX)/app/$(APP_NAME) -config ./$(CONFIG) & \
	(cd ui && pnpm dev) & \
	wait

build: ## Build the frontend and link the release binary into bin/
	cd ui && pnpm build
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(APP_NAME) $(PKG_PREFIX)/app/$(APP_NAME)

# Refreshes the three committed artifact sets, in dependency order:
#   pkg/db/query/*.sql          -> pkg/db/generated/            (sqlc)
#   pkg/apis registrations      -> app/*/apis/openapi.{json,yaml}
#   pkg/apis Go types           -> ui/src/generated/            (tygo)
# The perl steps exist because encoding/json flattens embedded structs
# while tygo emits them as a named field: inline TypeMeta to
# apiVersion?/kind?, and un-optionalize the ObjectMeta fields the server
# always sends. Generated code is committed -- `build` never regenerates.
generate: ## Regenerate committed sqlc / OpenAPI / TS artifacts (commit the result)
	cd pkg/db && sqlc generate
	go run $(PKG_PREFIX)/cmd/openapi-gen -apis-dir pkg/apis -output app/$(APP_NAME)/apis/openapi.json -format json
	go run $(PKG_PREFIX)/cmd/openapi-gen -apis-dir pkg/apis -output app/$(APP_NAME)/apis/openapi.yaml -format yaml
	go tool tygo generate
	perl -0pi -e 's/^([ \t]*)TypeMeta: TypeMeta;\n/$$1apiVersion?: string;\n$$1kind?: string;\n/gm' ui/src/generated/*.ts
	perl -0pi -e 's/^  (id|name|createdAt|updatedAt)\?: string;/  $$1: string;/mg' ui/src/generated/meta.ts

# Everything that must pass before a commit. `go test -race ./...` is the
# deeper pass (~10x slower); run it when touching concurrent code.
check: ## Run all gates: gofmt, vet, layer guard, Go tests, UI typecheck/lint/tests
	@out=$$(gofmt -l -s .); if [ -n "$$out" ]; then echo "gofmt -s needed:"; echo "$$out"; exit 1; fi
	go vet ./...
	./scripts/check-layer-leak.sh
	go test ./...
	cd ui && pnpm typecheck && pnpm lint && pnpm test

# The writing counterpart of `check`'s formatting gates (gofmt -l -s on the
# Go side, prettier --check inside `pnpm lint` on the UI side).
fmt: ## Format Go (gofmt -s) and the UI (prettier)
	gofmt -w -s .
	cd ui && pnpm format

# YYYYMMDDHHMMSS prefix, not YYYYMMDD: two migrations authored the same day
# would collide on the version number and Migrate() rejects duplicates.
# Never hand-create the file.
new-migration: ## Create a timestamped migration (NAME=add_widgets)
ifndef NAME
	$(error Usage: make new-migration NAME=description_here)
endif
	@f="pkg/db/migrations/$$(date -u +'%Y%m%d%H%M%S')_$(NAME).up.sql"; touch "$$f"; echo "$$f"

setup-hooks: ## Point git at .githooks (pre-commit: UI typecheck + layer guard)
	git config core.hooksPath .githooks
	@echo "core.hooksPath = .githooks"

clean: ## Remove build output
	rm -rf bin/
