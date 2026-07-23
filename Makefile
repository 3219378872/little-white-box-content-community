SHELL := /usr/bin/env bash

.DEFAULT_GOAL := help

ARGS ?=
FUZZ_TIME ?= 20s
INTEGRATION_PARALLELISM ?= 1
TEST_JSON_DIR ?=

export FUZZ_TIME INTEGRATION_PARALLELISM TEST_JSON_DIR

.PHONY: help fmt-check engineering-lint vet lint check test coverage coverage-target \
	coverage-no-gate integration-critical integration-init integration-run \
	integration-clear integration-all fuzz quality

help: ## Show the available project commands
	@printf '%s\n' 'Usage: make <target> [ARGS="..."]'
	@printf '%s\n' '' 'Checks:'
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z0-9_-]+:.*## / {printf "  %-24s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@printf '%s\n' '' 'Examples:'
	@printf '%s\n' '  make test ARGS="-run TestName -count=1"' \
		'  make lint ARGS="--timeout 5m"' \
		'  make fuzz FUZZ_TIME=10s'

fmt-check: ## Check formatting of handwritten Go files
	@unformatted="$$(git ls-files '*.go' | grep -vE '\.pb\.go$$|_grpc\.pb\.go$$|internal/types/types\.go$$|internal/handler/routes\.go$$' | xargs -r gofmt -l)"; \
	if [[ -n "$$unformatted" ]]; then \
		echo 'These files need gofmt:' >&2; \
		echo "$$unformatted" >&2; \
		exit 1; \
	fi

engineering-lint: ## Validate active docs, links, and repository policy
	python3 scripts/engineering-lint.py

vet: ## Run go vet across all workspace modules
	scripts/vet.sh $(ARGS)

lint: ## Run golangci-lint across all workspace modules
	scripts/lint.sh $(ARGS)

check: fmt-check engineering-lint vet lint ## Run formatting, policy, vet, and lint checks

test: ## Run race-enabled tests with package coverage across all modules
	scripts/test.sh $(ARGS)

coverage: ## Generate coverage reports and enforce the baseline gate
	scripts/coverage.sh $(ARGS)

coverage-target: ## Generate coverage reports and enforce the target gate
	scripts/coverage.sh --target $(ARGS)

coverage-no-gate: ## Generate coverage reports without enforcing a gate
	scripts/coverage.sh --no-gate $(ARGS)

integration-critical: ## Run the self-contained critical integration tests
	scripts/integration-test.sh --critical

integration-init: ## Start isolated DTM and SeaweedFS integration dependencies
	scripts/integration-env.sh init

integration-run: ## Run all integration tests against prepared dependencies
	scripts/integration-test.sh --all

integration-clear: ## Remove isolated integration dependencies
	scripts/integration-env.sh clear

integration-all: ## Start dependencies, run all integration tests, and always clean up
	scripts/integration-all.sh

fuzz: ## Run bounded native fuzz targets (override FUZZ_TIME as needed)
	scripts/fuzz.sh

quality: check test ## Run the standard local quality gates
