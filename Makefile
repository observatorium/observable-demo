PREFIX            ?= $(shell pwd)
FILES_TO_FMT      ?= $(shell find . -path ./vendor -prune -o -name '*.go' -print)

# Ensure everything works even if GOPATH is not set, which is often the case.
# Default to standard GOPATH.
GOPATH            ?= $(HOME)/go

GOBIN             ?= $(firstword $(subst :, ,${GOPATH}))/bin
GO111MODULE       ?= on
export GO111MODULE
GOPROXY           ?= https://proxy.golang.org
export GOPROXY

.PHONY: all
all: format build

.PHONY: build
build: deps
	@echo ">> building loadbalancer"
	@go build ./cmd/loadbalancer/...

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

# TODO: linting.

.PHONY: test
test:
	@echo ">> running tests"
	@go test ./...
