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

.PHONY: prepare deps tidy test vet fmt-check check

prepare:
	mkdir -p "$(GOCACHE)" "$(GOMODCACHE)" "$(GOPATH)" "$(TMPDIR)" \
		"$(COREPACK_HOME)" "$(PNPM_HOME)" "$(npm_config_store_dir)" \
		"$(NPM_CONFIG_CACHE)" "$(NPM_CONFIG_PREFIX)" "$(PLAYWRIGHT_BROWSERS_PATH)" \
		"$(XDG_CACHE_HOME)" "$(XDG_CONFIG_HOME)" "$(XDG_DATA_HOME)" "$(XDG_STATE_HOME)"

deps: prepare
	go mod download

tidy: prepare
	go mod tidy

test: prepare
	go test ./... -coverpkg=$(COVERPKG)

vet: prepare
	go vet ./...

fmt-check: prepare
	test -z "$$(gofmt -l $$(find . -path ./.git -prune -o -path ./.flowlens-dev -prune -o -name '*.go' -print))"

check: fmt-check vet test
