PKG_PREFIX := vraxel.io/vraxel
APP_NAME := vraxel-server
DATE_INFO_TAG ?= $(shell date -u +'%Y%m%d-%H%M%S')
BUILD_INFO_TAG ?= $(shell echo $$(git describe --long --all | tr '/' '-')$$( \
	      git diff-index --quiet HEAD -- || echo '-dirty-'$$(git diff-index -u HEAD | openssl sha1 | cut -d' ' -f2 | cut -c 1-8)))
RACE ?= -race
EXTRA_GO_BUILD_TAGS ?=
GO_BUILD_INFO = -X '$(PKG_PREFIX)/lib/buildinfo.Version=$(APP_NAME)-$(DATE_INFO_TAG)-$(BUILD_INFO_TAG)'

.PHONY: vraxel-server vraxel-server-prod build sqlc-generate openapi-gen ts-types test lint lint-layers fmt vet clean ui-install ui-dev ui-build ui-lint ui-test dev init-admin new-migration setup-hooks

setup-hooks:
	git config core.hooksPath .githooks
	@echo "Git hooks configured (pre-commit: TypeScript type check + layer-leak guard)"

lint-layers:
	@./scripts/check-layer-leak.sh

vraxel-server:
	CGO_ENABLED=1 go build $(RACE) -ldflags "$(GO_BUILD_INFO)" -tags "$(EXTRA_GO_BUILD_TAGS)" -o bin/$(APP_NAME)$(RACE) $(PKG_PREFIX)/app/$(APP_NAME)

vraxel-server-prod:
	CGO_ENABLED=0 go build -ldflags "$(GO_BUILD_INFO)" -tags "$(EXTRA_GO_BUILD_TAGS)" -o bin/$(APP_NAME) $(PKG_PREFIX)/app/$(APP_NAME)

# ./bin/vraxel-server -config ./app/vraxel-server/config.yaml
build: openapi-gen ui-build vraxel-server-prod

# YYYYMMDDHHMMSS prefix (not YYYYMMDD): two migrations authored the same
# day would otherwise collide on the version number and Migrate() rejects
# duplicates. Never hand-create the file.
new-migration:
ifndef NAME
	$(error Usage: make new-migration NAME=description_here)
endif
	@echo "pkg/db/migrations/$$(date -u +'%Y%m%d%H%M%S')_$(NAME).up.sql" && \
	touch "pkg/db/migrations/$$(date -u +'%Y%m%d%H%M%S')_$(NAME).up.sql"

sqlc-generate:
	cd pkg/db && sqlc generate

# Generate TypeScript types for the frontend from pkg/apis Go types
# (tygo, config in tygo.yaml). Output committed under ui/src/generated/.
# The perl step inlines embedded TypeMeta fields to apiVersion?/kind?
# (encoding/json flattens embedded structs).
ts-types:
	go tool tygo generate
	perl -0pi -e 's/^([ \t]*)TypeMeta: TypeMeta;\n/$$1apiVersion?: string;\n$$1kind?: string;\n/gm' ui/src/generated/*.ts
	perl -0pi -e 's/^  (id|name|createdAt|updatedAt)\?: string;/  $$1: string;/mg' ui/src/generated/meta.ts

openapi-gen:
	go run $(PKG_PREFIX)/cmd/openapi-gen -apis-dir pkg/apis -output app/vraxel-server/apis/openapi.json -format json
	go run $(PKG_PREFIX)/cmd/openapi-gen -apis-dir pkg/apis -output app/vraxel-server/apis/openapi.yaml -format yaml

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -w -s .

init-admin:
	go run $(PKG_PREFIX)/cmd/init-admin

clean:
	rm -rf bin/

ui-install:
	cd ui && pnpm install

ui-dev:
	cd ui && pnpm dev

ui-build:
	cd ui && pnpm build

ui-lint:
	cd ui && pnpm lint

ui-test:
	cd ui && pnpm test

# Prefers the gitignored per-developer overlay (config.dev.yaml) when it
# exists; otherwise the committed config.yaml, whose defaults boot
# against localhost PG with auth on.
DEV_CONFIG := $(if $(wildcard app/$(APP_NAME)/config.dev.yaml),app/$(APP_NAME)/config.dev.yaml,app/$(APP_NAME)/config.yaml)

dev:
	@trap 'kill 0' EXIT; \
	go run $(PKG_PREFIX)/app/$(APP_NAME) -config ./$(DEV_CONFIG) & \
	cd ui && pnpm dev & \
	wait
