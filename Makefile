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
PWD ?= $(shell pwd)

.PHONY: all
all: format build

.PHONY: build
build: deps
	@echo ">> building loadbalancer"
	@go build -a -tags netgo -ldflags '${LDFLAGS}' ./cmd/loadbalancer/...

.PHONY: docker
docker: build
	@echo ">> build docker"
	@docker build -t observable-demo-image .

.PHONY: demo
demo: docker
	@docker rm -f observable-demo || true
	@echo ">> running Prometheus. Go to browser on http://localhost:9090 for Prometheus UI"
	@docker run -d -v $(PWD)/demo-prometheus.yml:/etc/prometheus/prometheus.yml -p 8080:8080 -p 9090:9090 --name observable-demo observable-demo-image
	@echo ">> running demo. Go to browser for http://localhost:8080/lb for triggering actions"
	@docker exec -it observable-demo /bin/loadbalancer \
	 --listen-address=:8080 \
	 --listen-demo1-address=localhost:8081 \
	 --listen-demo2-address=localhost:8082 \
	 --listen-demo3-address=localhost:8083 \
	 --targets=http://localhost:8081,http://localhost:8082,http://localhost:8083 || true
	@docker kill observable-demo

.PHONY: demo-test
demo-test:
	@echo ">> calling loadbalancer"
	@while sleep 0.5; do curl -s http://localhost:8080/lb; done

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
