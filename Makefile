PKG_PREFIX := vraxel.io/vraxel
APP_NAME   := vraxel-server

# Stamped into lib/buildinfo.Version; "dev" outside a git tree.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X '$(PKG_PREFIX)/lib/buildinfo.Version=$(APP_NAME)-$(VERSION)'
# The agent names itself. Both binaries share lib/buildinfo, so building
# the agent with LDFLAGS stamped it "vraxel-server-<ver>" -- which is what
# host_agents.version, the host detail page and the install summary all
# then reported as the agent's version.
AGENT_LDFLAGS := -X '$(PKG_PREFIX)/lib/buildinfo.Version=vr-agent-$(VERSION)'

# The gitignored per-developer overlay wins when present; otherwise the
# committed config, whose defaults boot against localhost PG with
# authentication on.
CONFIG := $(if $(wildcard app/$(APP_NAME)/config.dev.yaml),app/$(APP_NAME)/config.dev.yaml,app/$(APP_NAME)/config.yaml)

.DEFAULT_GOAL := help
.PHONY: help dev build agent-binaries generate quick check fmt clean new-migration setup-hooks

help: ## List available commands
	@awk 'BEGIN{FS=":.*## "} /^[a-z][a-z-]*:.*## /{printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

dev: agent-binaries ## Run vraxel-server (:9099) + vite (:5199) with HMR
	@echo "config: $(CONFIG)"
	@trap 'kill 0' EXIT; \
	go run $(PKG_PREFIX)/app/$(APP_NAME) -config ./$(CONFIG) & \
	(cd ui && pnpm dev) & \
	wait

build: agent-binaries ## Build the frontend and link the release binary into bin/
	cd ui && pnpm build
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(APP_NAME) $(PKG_PREFIX)/app/$(APP_NAME)

# The binaries /api/agent/v1/binary/{os}/{arch} serves, and whose digests
# install-agent.sh states. Static (CGO_ENABLED=0) because the target is
# whatever minimal image the customer happens to run; the server hashes
# whatever is in bin/ at request time, so rebuilding here is enough to
# change what hosts install.
#
# A prerequisite of both `dev` and `build`, because nothing else rebuilds
# them and a stale one is silent: the agent still installs, still comes
# online, and merely omits whatever the server learned to ask for since.
# That cost a debugging session -- an agent built before machine
# fingerprinting reported none, so every reinstall failed to match its own
# host row and split off a duplicate for an operator to merge.
#
# File targets, not a phony recipe, so the common case (source unchanged)
# is a stat and the cross-compile only runs when it would produce
# something different.
AGENT_BINS := bin/vr-agent-linux-amd64 bin/vr-agent-linux-arm64

# What the agent is built from. `go list -deps ./app/vr-agent` reaches
# app/vr-agent and lib/ only, so these two trees plus the module files are
# the whole input: listing less would miss a rebuild, listing more only
# costs an occasional needless one.
AGENT_SRC := $(shell find app/vr-agent lib -name '*.go') go.mod go.sum

agent-binaries: $(AGENT_BINS) ## Cross-compile vr-agent into bin/ for the targets the server serves

$(AGENT_BINS): bin/vr-agent-linux-%: $(AGENT_SRC)
	CGO_ENABLED=0 GOOS=linux GOARCH=$* \
	  go build -ldflags "$(AGENT_LDFLAGS)" -o $@ $(PKG_PREFIX)/app/vr-agent

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
quick: ## Fast pre-commit check: gate integrity + only what changed
	@./scripts/quick-check.sh

check: ## Run all gates: gofmt, vet, layer guard, Go tests, UI typecheck/lint/tests
	@out=$$(gofmt -l -s . | grep -vE "^\.worktrees/|^\.anvil-dev/" || true); if [ -n "$$out" ]; then echo "gofmt -s needed:"; echo "$$out"; exit 1; fi
	go vet ./...
	./scripts/check-layer-leak.sh
	go test ./...
	node ./scripts/check-lint-config.mjs
	pnpm -r typecheck
	pnpm -r lint
	pnpm -r --if-present test

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
