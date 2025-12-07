# Makefile for sadp - Hikvision Device Discovery Tool

ROOT_FOLDER         := $(shell git rev-parse --show-toplevel)
include             $(ROOT_FOLDER)/.scripts/golang.mk
include             $(ROOT_FOLDER)/.scripts/markdown.mk

SHA1                := $(shell git rev-parse --verify HEAD)
SHA1_SHORT          := $(shell git rev-parse --verify --short HEAD)
PWD                 := $(shell pwd)

# Project settings
PROJECT             ?= sadp
BINARY              := sadp
MAIN_PATH           := ./cmd/sadp
MODULE              := github.com/cameronnewman/hikvision-tooling

#
# Default Goals
#
.DEFAULT_GOAL       := help

# HELP
# This will output the help for each task
# thanks to https://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
.PHONY: help
help: ## Returns a list of all the make goals
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: version
version: ## Returns version for build
	@echo "Build Version: v$(VERSION_HASH)"

#
# Alias targets (map to go-* targets from golang.mk)
#

.PHONY: build
build: go-build ## Build the binary

.PHONY: install
install: go-install ## Install binary to GOBIN

.PHONY: release
release: go-release ## Build for all platforms

.PHONY: test
test: go-test ## Run tests

.PHONY: test-cover
test-cover: go-test-cover ## Run tests with coverage

.PHONY: test-cover-html
test-cover-html: go-test-cover-html ## Generate HTML coverage report

.PHONY: lint
lint: go-lint ## Run linters

.PHONY: lint-fix
lint-fix: go-lint-fix ## Run linters with auto-fix

.PHONY: fmt
fmt: go-fmt ## Format Go code

.PHONY: fmt-check
fmt-check: go-fmt-check ## Check if code is formatted

.PHONY: mod
mod: go-mod ## Run go mod tidy

.PHONY: vet
vet: go-vet ## Run go vet

.PHONY: clean
clean: go-clean ## Remove build artifacts

.PHONY: check
check: fmt-check vet lint test ## Run all checks

.PHONY: run
run: build ## Build and run
	./$(BUILD_DIR)/$(BINARY)
