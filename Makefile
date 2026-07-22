SHELL := /bin/sh
PROJECT_ROOT := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
DEV_ROOT := $(PROJECT_ROOT)/.flowlens-dev
COVERPKG := github.com/Willxup/flowlens/...

export GOENV := off
export GOTOOLCHAIN := auto
export GOTELEMETRY := off
export GOCACHE := $(DEV_ROOT)/cache/go-build
export GOMODCACHE := $(DEV_ROOT)/cache/go-mod
export GOPATH := $(DEV_ROOT)/cache/go-path
export TMPDIR := $(DEV_ROOT)/tmp
export COREPACK_HOME := $(DEV_ROOT)/cache/corepack
export PNPM_HOME := $(DEV_ROOT)/cache/pnpm/home
export npm_config_store_dir := $(DEV_ROOT)/cache/pnpm/store
export NPM_CONFIG_CACHE := $(DEV_ROOT)/cache/npm
export NPM_CONFIG_PREFIX := $(DEV_ROOT)/tools/npm
export PLAYWRIGHT_BROWSERS_PATH := $(DEV_ROOT)/cache/playwright
export XDG_CACHE_HOME := $(DEV_ROOT)/cache/xdg
export XDG_CONFIG_HOME := $(DEV_ROOT)/config/xdg
export XDG_DATA_HOME := $(DEV_ROOT)/data/xdg
export XDG_STATE_HOME := $(DEV_ROOT)/state/xdg

.PHONY: prepare deps frontend-deps playwright-deps frontend-format-check frontend-typecheck frontend-test frontend-e2e frontend-build frontend-check tidy test vet fmt-check check release-check release-image release-multiarch

PNPM_INSTALL_FLAGS ?= --frozen-lockfile
VERSION ?= dev
COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null)
BUILD_DATE ?= $(shell git show -s --format=%cI HEAD 2>/dev/null)
IMAGE ?= flowlens:$(VERSION)
PLATFORMS ?= linux/amd64,linux/arm64
PUSH ?= false
GITLEAKS ?= gitleaks
REPORT_DIR := $(DEV_ROOT)/reports

ifeq ($(PUSH),true)
MULTIARCH_OUTPUT := --push
else
MULTIARCH_OUTPUT := --output type=oci,dest=$(REPORT_DIR)/flowlens-$(VERSION).oci.tar
endif

prepare:
	mkdir -p "$(GOCACHE)" "$(GOMODCACHE)" "$(GOPATH)" "$(TMPDIR)" \
		"$(COREPACK_HOME)" "$(PNPM_HOME)" "$(npm_config_store_dir)" \
		"$(NPM_CONFIG_CACHE)" "$(NPM_CONFIG_PREFIX)" "$(PLAYWRIGHT_BROWSERS_PATH)" "$(DEV_ROOT)/cache/typescript" \
		"$(XDG_CACHE_HOME)" "$(XDG_CONFIG_HOME)" "$(XDG_DATA_HOME)" "$(XDG_STATE_HOME)"

deps: prepare frontend-deps
	go mod download

frontend-deps: prepare
	pnpm --dir web install $(PNPM_INSTALL_FLAGS)

playwright-deps: prepare
	pnpm --dir web exec playwright install chromium

frontend-format-check: prepare
	pnpm --dir web format:check

frontend-typecheck: prepare
	pnpm --dir web typecheck

frontend-test: prepare
	pnpm --dir web test:run

frontend-e2e: prepare
	pnpm --dir web test:e2e

frontend-build: prepare
	pnpm --dir web build:app

frontend-check: frontend-format-check frontend-typecheck frontend-test frontend-build

tidy: prepare
	go mod tidy

test: prepare
	go test ./... -coverpkg=$(COVERPKG)

vet: prepare
	go vet ./...

fmt-check: prepare
	test -z "$$(gofmt -l $$(find . -path ./.git -prune -o -path ./.flowlens-dev -prune -o -name '*.go' -print))"

check: frontend-check fmt-check vet test

release-check: prepare
	go test ./test/release -count=1
	go mod verify
	mkdir -p "$(REPORT_DIR)"
	git ls-files --cached --others --exclude-standard -z > "$(REPORT_DIR)/release-files.zlist"
	tar --null -T "$(REPORT_DIR)/release-files.zlist" -cf "$(REPORT_DIR)/release-tree.tar"
	@if ! command -v "$(GITLEAKS)" >/dev/null 2>&1; then \
		echo "gitleaks is required for release-check"; \
		exit 1; \
	fi
	"$(GITLEAKS)" dir --max-archive-depth=1 --config .gitleaks.toml --no-banner --redact "$(REPORT_DIR)/release-tree.tar"

release-image: prepare
	docker build \
		--build-arg VERSION="$(VERSION)" \
		--build-arg COMMIT="$(COMMIT)" \
		--build-arg BUILD_DATE="$(BUILD_DATE)" \
		--tag "$(IMAGE)" .

release-multiarch: prepare
	mkdir -p "$(REPORT_DIR)"
	docker buildx build \
		--platform "$(PLATFORMS)" \
		--build-arg VERSION="$(VERSION)" \
		--build-arg COMMIT="$(COMMIT)" \
		--build-arg BUILD_DATE="$(BUILD_DATE)" \
		--tag "$(IMAGE)" $(MULTIARCH_OUTPUT) .
