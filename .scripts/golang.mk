# golang.mk - Go build targets with Docker support
#
# Environment:
#   ENVIRONMENT=local   - Run commands directly
#   ENVIRONMENT=docker  - Run commands directly
#   ENVIRONMENT=CI      - Run commands in Docker (default)

# Docker settings
DOCKER ?= docker
GOLANG_BUILD_IMAGE ?= docker.io/library/golang:1.21-bullseye
GOLANG_LINT_IMAGE := docker.io/golangci/golangci-lint:v1.61.0

# Environment: local, docker, or CI (default: CI runs in docker)
ENVIRONMENT ?= CI

# Go settings
GO ?= go
GOFLAGS ?=
CGO_ENABLED ?= 0

# Build settings
BUILD_DIR := bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
VERSION_HASH := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-s -w -X main.Version=$(VERSION) -X main.Commit=$(VERSION_HASH) -X main.BuildTime=$(BUILD_TIME)"

# Platform detection
GOOS ?= $(shell $(GO) env GOOS)
GOARCH ?= $(shell $(GO) env GOARCH)

# Test settings
COVERAGE_DIR := coverage
COVERAGE_FILE := $(COVERAGE_DIR)/coverage.out
COVERAGE_HTML := $(COVERAGE_DIR)/coverage.html

# Platforms for release builds
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

## Build targets

.PHONY: go-build
go-build: ## Build the binary for current platform
	@echo "+++ $(shell date) - Running 'go build'"

ifeq ($(filter $(ENVIRONMENT),local docker),$(ENVIRONMENT))
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) $(MAIN_PATH)
else
	@mkdir -p $(BUILD_DIR)
	DOCKER_BUILDKIT=1 \
	$(DOCKER) run --rm \
	-v $(PWD):/usr/src/app \
	-w /usr/src/app \
	--entrypoint=bash \
	$(GOLANG_BUILD_IMAGE) \
	-c "CGO_ENABLED=$(CGO_ENABLED) GOOS=linux GOARCH=amd64 go build -buildvcs=false $(GOFLAGS) -ldflags '-s -w -X main.Version=$(VERSION) -X main.Commit=$(VERSION_HASH)' -o $(BUILD_DIR)/$(BINARY) $(MAIN_PATH)"
endif

	@echo "$(shell date) - Completed 'go build': $(BUILD_DIR)/$(BINARY)"

.PHONY: go-install
go-install: ## Install binary to GOBIN (local only)
	@echo "+++ $(shell date) - Running 'go install'"
	CGO_ENABLED=$(CGO_ENABLED) $(GO) install $(GOFLAGS) $(LDFLAGS) $(MAIN_PATH)
	@echo "$(shell date) - Completed 'go install'"

.PHONY: go-release
go-release: ## Build for all platforms
	@echo "+++ $(shell date) - Building release binaries..."

ifeq ($(filter $(ENVIRONMENT),local docker),$(ENVIRONMENT))
	@mkdir -p $(BUILD_DIR)
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		output=$(BUILD_DIR)/$(BINARY)-$$os-$$arch; \
		if [ "$$os" = "windows" ]; then output="$$output.exe"; fi; \
		echo "Building $$os/$$arch..."; \
		CGO_ENABLED=$(CGO_ENABLED) GOOS=$$os GOARCH=$$arch $(GO) build $(GOFLAGS) $(LDFLAGS) -o $$output $(MAIN_PATH); \
	done
else
	@mkdir -p $(BUILD_DIR)
	DOCKER_BUILDKIT=1 \
	$(DOCKER) run --rm \
	-v $(PWD):/usr/src/app \
	-w /usr/src/app \
	--entrypoint=bash \
	$(GOLANG_BUILD_IMAGE) \
	-c 'for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		output=$(BUILD_DIR)/$(BINARY)-$$os-$$arch; \
		if [ "$$os" = "windows" ]; then output="$$output.exe"; fi; \
		echo "Building $$os/$$arch..."; \
		CGO_ENABLED=$(CGO_ENABLED) GOOS=$$os GOARCH=$$arch go build -buildvcs=false -ldflags "-s -w -X main.Version=$(VERSION) -X main.Commit=$(VERSION_HASH)" -o $$output $(MAIN_PATH); \
	done'
endif

	@echo "$(shell date) - Completed release builds in $(BUILD_DIR)/"

## Test targets

.PHONY: go-test
go-test: ## Run tests
	@echo "+++ $(shell date) - Running 'go test'"

ifeq ($(filter $(ENVIRONMENT),local docker),$(ENVIRONMENT))
	$(GO) test -race -failfast -v ./...
else
	DOCKER_BUILDKIT=1 \
	$(DOCKER) run --rm \
	-v $(PWD):/usr/src/app \
	-w /usr/src/app \
	--entrypoint=bash \
	$(GOLANG_BUILD_IMAGE) \
	-c "go test -race -failfast -v ./..."
endif

	@echo "+++ $(shell date) - Completed 'go test'"

.PHONY: go-test-cover
go-test-cover: ## Run tests with coverage
	@echo "+++ $(shell date) - Running 'go test' with coverage"

ifeq ($(filter $(ENVIRONMENT),local docker),$(ENVIRONMENT))
	@mkdir -p $(COVERAGE_DIR)
	$(GO) test -race -failfast -cover -coverprofile=$(COVERAGE_FILE) -covermode=atomic -v ./...
	$(GO) tool cover -func=$(COVERAGE_FILE)
else
	@mkdir -p $(COVERAGE_DIR)
	DOCKER_BUILDKIT=1 \
	$(DOCKER) run --rm \
	-v $(PWD):/usr/src/app \
	-w /usr/src/app \
	--entrypoint=bash \
	$(GOLANG_BUILD_IMAGE) \
	-c "go test -race -failfast -cover -coverprofile=$(COVERAGE_FILE) -covermode=atomic -v ./... && go tool cover -func=$(COVERAGE_FILE)"
endif

	@echo "+++ $(shell date) - Completed 'go test' with coverage"

.PHONY: go-test-cover-html
go-test-cover-html: go-test-cover ## Generate HTML coverage report (local only)
	@echo "+++ $(shell date) - Generating HTML coverage report..."
	$(GO) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "Coverage report: $(COVERAGE_HTML)"

.PHONY: go-test-short
go-test-short: ## Run short tests only
	@echo "+++ $(shell date) - Running 'go test -short'"

ifeq ($(filter $(ENVIRONMENT),local docker),$(ENVIRONMENT))
	$(GO) test -short ./...
else
	DOCKER_BUILDKIT=1 \
	$(DOCKER) run --rm \
	-v $(PWD):/usr/src/app \
	-w /usr/src/app \
	--entrypoint=bash \
	$(GOLANG_BUILD_IMAGE) \
	-c "go test -short ./..."
endif

	@echo "+++ $(shell date) - Completed 'go test -short'"

## Lint targets

.PHONY: go-lint
go-lint: ## Run golangci-lint
	@echo "+++ $(shell date) - Running 'golangci-lint run'"

ifeq ($(filter $(ENVIRONMENT),local docker),$(ENVIRONMENT))
	golangci-lint run -v ./...
else
	DOCKER_BUILDKIT=1 \
	$(DOCKER) run --rm \
	-e GOPACKAGESPRINTGOLISTERRORS=1 \
	-e GO111MODULE=on \
	-e GOGC=100 \
	-v $(PWD):/usr/src/app \
	-w /usr/src/app \
	--entrypoint "/bin/bash" \
	$(GOLANG_LINT_IMAGE) \
	-c "golangci-lint run -v ./..."
endif

	@echo "$(shell date) - Completed 'golangci-lint run'"

.PHONY: go-lint-fix
go-lint-fix: ## Run linters with auto-fix (local only)
	@echo "+++ $(shell date) - Running 'golangci-lint run --fix'"
	golangci-lint run --fix ./...
	@echo "$(shell date) - Completed 'golangci-lint run --fix'"

## Format targets

.PHONY: go-fmt
go-fmt: ## Format Go code
	@echo "+++ $(shell date) - Running 'go fmt'"

ifeq ($(filter $(ENVIRONMENT),local docker),$(ENVIRONMENT))
	$(GO) fmt ./...
else
	DOCKER_BUILDKIT=1 \
	$(DOCKER) run --rm \
	-v $(PWD):/usr/src/app \
	-w /usr/src/app \
	--entrypoint "/bin/bash" \
	$(GOLANG_BUILD_IMAGE) \
	-c "go fmt ./..."
endif

	@echo "$(shell date) - Completed 'go fmt'"

.PHONY: go-fmt-check
go-fmt-check: ## Check if code is formatted
	@echo "+++ $(shell date) - Checking code formatting..."
	@test -z "$$(gofmt -l .)" || (echo "Code is not formatted. Run 'make fmt'" && exit 1)
	@echo "$(shell date) - Code formatting check passed"

## Dependency targets

.PHONY: go-mod
go-mod: ## Run go mod tidy
	@echo "+++ $(shell date) - Running 'go mod tidy'"

ifeq ($(filter $(ENVIRONMENT),local docker),$(ENVIRONMENT))
	$(GO) mod tidy
else
	DOCKER_BUILDKIT=1 \
	$(DOCKER) run --rm \
	-v $(PWD):/usr/src/app \
	-w /usr/src/app \
	--entrypoint "/bin/bash" \
	$(GOLANG_BUILD_IMAGE) \
	-c "go mod tidy"
endif

	@echo "$(shell date) - Completed 'go mod tidy'"

.PHONY: go-deps
go-deps: ## Download dependencies
	@echo "+++ $(shell date) - Downloading dependencies..."
	$(GO) mod download
	@echo "$(shell date) - Completed downloading dependencies"

.PHONY: go-deps-verify
go-deps-verify: ## Verify dependencies
	@echo "+++ $(shell date) - Verifying dependencies..."
	$(GO) mod verify
	@echo "$(shell date) - Completed verifying dependencies"

## Vet target

.PHONY: go-vet
go-vet: ## Run go vet
	@echo "+++ $(shell date) - Running 'go vet'"

ifeq ($(filter $(ENVIRONMENT),local docker),$(ENVIRONMENT))
	$(GO) vet ./...
else
	DOCKER_BUILDKIT=1 \
	$(DOCKER) run --rm \
	-v $(PWD):/usr/src/app \
	-w /usr/src/app \
	--entrypoint "/bin/bash" \
	$(GOLANG_BUILD_IMAGE) \
	-c "go vet ./..."
endif

	@echo "$(shell date) - Completed 'go vet'"

## Clean targets

.PHONY: go-clean
go-clean: ## Remove build artifacts
	@echo "+++ $(shell date) - Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -rf $(COVERAGE_DIR)
	@$(GO) clean
	@echo "$(shell date) - Completed cleaning"

## Docker targets

.PHONY: go-docker-bash
go-docker-bash: ## Start an interactive shell in the golang docker image
	DOCKER_BUILDKIT=1 \
	$(DOCKER) run -it --rm \
	-v $(PWD):/usr/src/app \
	-w /usr/src/app \
	--entrypoint "/bin/bash" \
	$(GOLANG_BUILD_IMAGE)
