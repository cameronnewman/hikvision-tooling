# markdown.mk - Markdown linting targets
#
# Uses markdownlint-cli (Node-based markdown linter)
# https://www.npmjs.com/package/markdownlint-cli

# Docker settings
DOCKER ?= docker
MARKDOWN_LINT_IMAGE := docker.io/tmknom/markdownlint:0.23.1

## Markdown lint targets

.PHONY: markdown-lint
markdown-lint: ## Runs markdown lint for consistency.
	@echo "+++ $$(date) - Running markdown lint"

	DOCKER_BUILDKIT=1 \
	$(DOCKER) run --rm \
	-v $(PWD):/work \
	-w /work \
	$(MARKDOWN_LINT_IMAGE)

	@echo "$$(date) - Completed markdown Lint"
