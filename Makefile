SHELL := /bin/sh

.DEFAULT_GOAL := help

.PHONY: bootstrap build check clean dev fmt generate help test test-e2e test-go test-web

help:
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "%-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

bootstrap: ## Install Go and web dependencies.
	go mod download
	pnpm install --frozen-lockfile

generate: ## Regenerate Go and TypeScript API models.
	go tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml
	pnpm --dir web generate:api

fmt: ## Format Go and web source.
	gofmt -w $$(find cmd internal -name '*.go' -type f)
	pnpm --dir web format

test-go: ## Run Go tests.
	go test ./...

test-web: ## Run web tests.
	pnpm --dir web test

test: test-go test-web ## Run all tests.

test-e2e: ## Run fixture-driven desktop and mobile browser tests.
	./scripts/e2e.sh

build: ## Build the binary and web application.
	go build ./cmd/tailpath
	pnpm --dir web build

check: ## Run generated-file, formatting, test, and build checks.
	./scripts/check-generated.sh
	sh -n scripts/select-edge-tag.sh scripts/tests/select-edge-tag.sh
	./scripts/tests/select-edge-tag.sh
	test -z "$$(gofmt -l $$(find cmd internal -name '*.go' -type f))"
	go vet ./...
	go test ./...
	pnpm --dir web check
	pnpm --dir web test
	pnpm --dir web build
	./scripts/e2e.sh

dev: ## Run the fixture API and Vite development server.
	./scripts/dev.sh

clean: ## Remove local build output.
	rm -rf web/dist tailpath
