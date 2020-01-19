PREFIX            ?= $(shell pwd)
FILES_TO_FMT      ?= $(shell find . -path ./vendor -prune -o -name '*.go' -print)

VERSION := $(strip $(shell [ -d .git ] && git describe --always --tags --dirty))
BUILD_DATE := $(shell date -u +"%Y-%m-%d")
BUILD_TIMESTAMP := $(shell date -u +"%Y-%m-%dT%H:%M:%S%Z")
VCS_REF := $(strip $(shell [ -d .git ] && git rev-parse --short HEAD))
BRANCH := $(strip $(shell git rev-parse --abbrev-ref HEAD))
VERSION := $(strip $(shell [ -d .git ] && git describe --always --tags --dirty))
USER ?= $(shell id -u -n)
HOST ?= $(shell hostname)
LDFLAGS := -s -w \
	-X github.com/prometheus/common/version.Version="$(VERSION)" \
	-X github.com/prometheus/common/version.Revision="$(VCS_REF)" \
	-X github.com/prometheus/common/version.Branch="$(BRANCH)" \
	-X github.com/prometheus/common/version.BuildUser="${USER}"@"${HOST}" \
	-X github.com/prometheus/common/version.BuildDate="$(BUILD_DATE)" \

# Ensure everything works even if GOPATH is not set, which is often the case.
# Default to standard GOPATH.
FIRST_GOPATH := $(firstword $(subst :, ,$(shell go env GOPATH)))

GOBIN             ?= $(firstword $(subst :, ,${FIRST_GOPATH}))/bin
GO111MODULE       ?= on
export GO111MODULE
GOPROXY           ?= https://proxy.golang.org
export GOPROXY

OS ?= $(shell uname -s | tr '[A-Z]' '[a-z]')
ARCH ?= $(shell uname -m)
BIN_DIR ?= ./tmp/bin
GOLANGCILINT ?= $(FIRST_GOPATH)/bin/golangci-lint
GOLANGCILINT_VERSION ?= v1.21.0
SHELLCHECK ?= $(BIN_DIR)/shellcheck

.PHONY: all
all: format build

.PHONY: build
build: deps
	@echo ">> building loadbalancer"
	@go build -ldflags '${LDFLAGS}' ./cmd/loadbalancer/...

# deps ensures fresh go.mod and go.sum.
.PHONY: deps
deps:
	@go mod tidy
	@go mod verify

# format formats the code (including imports format).
.PHONY: format
format: $(GOIMPORTS)
	@echo ">> formatting code"
	@goimports -w $(FILES_TO_FMT)

.PHONY: lint
lint: $(GOLANGCILINT) $(SHELLCHECK)
	$(GOLANGCILINT) run -v --enable-all -c .golangci.yml

.PHONY: fix
fix: $(GOLANGCILINT)
	$(GOLANGCILINT) run --fix --enable-all -c .golangci.yml

.PHONY: test
test:
	@echo ">> running tests"
	@go test ./...

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

$(GOLANGCILINT):
	@echo "Downloading Golangci-lint"
	curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/$(GOLANGCILINT_VERSION)/install.sh \
		| sed -e '/install -d/d' \
		| sh -s -- -b $(FIRST_GOPATH)/bin $(GOLANGCILINT_VERSION)

$(SHELLCHECK): $(BIN_DIR)
	@echo "Downloading Shellcheck"
	curl -sNL "https://storage.googleapis.com/shellcheck/shellcheck-stable.$(OS).$(ARCH).tar.xz" | tar --strip-components=1 -xJf - -C $(BIN_DIR)
