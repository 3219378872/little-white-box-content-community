SHELL := /usr/bin/env bash

.DEFAULT_GOAL := help

ARGS ?=
FUZZ_TIME ?= 20s
INTEGRATION_PARALLELISM ?= 1
TEST_JSON_DIR ?=
PERF_GATEWAY_BASE_URL ?= http://127.0.0.1:8888
PERF_GATEWAY_CONCURRENCY ?= 8
PERF_GATEWAY_REQUESTS ?= 20
PERF_GATEWAY_SCENARIOS ?= behavior,search,feed,gateway,assistant
SEARCH_REBUILD_CONFIG ?= app/search/mq/etc/search-consumer.yaml
EMBEDDING_REBUILD_CONFIG ?= app/embedding/mq/etc/embedding-consumer.yaml
PRODUCTION_ENV_FILE ?=
PRODUCTION_COMPOSE = docker compose $(if $(PRODUCTION_ENV_FILE),--env-file $(PRODUCTION_ENV_FILE),) \
	-f deploy/docker-compose.middleware.yml -f deploy/docker-compose.production.yml --profile production
PRODUCTION_BUILD_ARGS ?= --build-arg HTTP_PROXY --build-arg HTTPS_PROXY \
	--build-arg NO_PROXY --build-arg ALL_PROXY

export FUZZ_TIME INTEGRATION_PARALLELISM TEST_JSON_DIR

.PHONY: help generate fmt-check engineering-lint vet lint check test coverage coverage-target \
	coverage-no-gate integration-critical integration-init integration-run \
	integration-clear integration-all fuzz quality search-rebuild embedding-rebuild \
	algorithm-test spec-evals-test model-pipeline-integration performance-gateway \
	fault-injection-recommend production-config production-build \
	production-up production-down gen-frozen-evals gen-recommend-samples gen-slo-synthetic

help: ## Show the available project commands
	@printf '%s\n' 'Usage: make <target> [ARGS="..."]'
	@printf '%s\n' '' 'Checks:'
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z0-9_-]+:.*## / {printf "  %-24s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@printf '%s\n' '' 'Examples:'
	@printf '%s\n' '  make test ARGS="-run TestName -count=1"' \
		'  make lint ARGS="--timeout 5m"' \
		'  make fuzz FUZZ_TIME=10s'

generate: ## Regenerate API, protobuf, and RPC code
	scripts/generate.sh

fmt-check: ## Check formatting of handwritten Go files
	@unformatted="$$(git ls-files --cached --others --exclude-standard '*.go' | grep -vE '\.pb\.go$$|_grpc\.pb\.go$$|internal/types/types\.go$$|internal/handler/routes\.go$$' | while IFS= read -r file; do [[ -f "$$file" ]] && printf '%s\0' "$$file"; done | xargs -0 -r gofmt -l)"; \
	if [[ -n "$$unformatted" ]]; then \
		echo 'These files need gofmt:' >&2; \
		echo "$$unformatted" >&2; \
		exit 1; \
	fi

engineering-lint: ## Validate layered knowledge, links, and repository policy
	python3 -m unittest discover -s scripts -p 'test_engineering_lint.py'
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

integration-init: ## Start isolated SeaweedFS integration dependency
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

gen-frozen-evals: ## Regenerate frozen eval datasets (corpus/qrels/cases) via LLM
	python3 scripts/gen_frozen_evals.py $(ARGS)

gen-recommend-samples: ## Regenerate the frozen recommendation sample set via LLM
	python3 scripts/gen_recommend_samples.py $(ARGS)

gen-slo-synthetic: ## Regenerate the deterministic synthetic SLO observations
	python3 scripts/gen_slo_synthetic.py $(ARGS)

algorithm-test: ## Run dependency-light Python algorithm unit tests
	python3 -m unittest discover -s algorithm -p 'test*.py' -v

spec-evals-test: ## Run the frozen spec-quality gate evaluator unit tests
	cd scripts && python3 -m unittest -v test_spec_evals.py

model-pipeline-integration: ## Verify ClickHouse, LightGBM, MinIO, and OnlineInfer end to end
	algorithm/integration/run.sh

performance-gateway: ## Check live Gateway P95 targets (JWT via PERF_GATEWAY_TOKEN)
	python3 -m unittest -q scripts/test_gateway_performance.py
	python3 scripts/gateway_performance.py --base-url "$(PERF_GATEWAY_BASE_URL)" \
		--scenarios "$(PERF_GATEWAY_SCENARIOS)" --concurrency "$(PERF_GATEWAY_CONCURRENCY)" \
		--requests "$(PERF_GATEWAY_REQUESTS)" $(ARGS)

fault-injection-recommend: ## Verify OnlineInfer timeout/outage rule fallback
	go test -race -count=1 -run '^TestGetRecommendPostsFaultInjectionOnlineInfer$$' \
		./app/recommend/rpc/internal/logic

search-rebuild: ## Rebuild the post search index and atomically promote its alias
	go run ./app/search/mq/cmd/rebuild -f $(SEARCH_REBUILD_CONFIG)

embedding-rebuild: ## Rebuild post embeddings and atomically promote the Milvus alias
	go run ./app/embedding/mq/cmd/rebuild -f $(EMBEDDING_REBUILD_CONFIG)

production-config: ## Render and validate the production Compose project
	$(PRODUCTION_COMPOSE) config --quiet

production-build: ## Build production Go and algorithm service images
	$(PRODUCTION_COMPOSE) build $(PRODUCTION_BUILD_ARGS)

production-up: ## Start the production-like stack
	$(PRODUCTION_COMPOSE) build $(PRODUCTION_BUILD_ARGS)
	$(PRODUCTION_COMPOSE) up -d

production-down: ## Stop the production-like stack without deleting volumes
	$(PRODUCTION_COMPOSE) down
