VERSION ?= dev
COMMIT  := $(shell git rev-parse --short=7 HEAD 2>/dev/null)$(shell git diff --quiet 2>/dev/null || echo '-dirty')
DATE    := $(shell date -u +%Y-%m-%d)
LDFLAGS := -X github.com/omarkohl/jip/cmd.Version=$(VERSION) \
           -X github.com/omarkohl/jip/cmd.Commit=$(COMMIT) \
           -X github.com/omarkohl/jip/cmd.Date=$(DATE)

.PHONY: help build test test-integration lint fmt check clean

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

build: ## Build the binary
	go build -ldflags '$(LDFLAGS)' -o jip .

test: ## Run unit tests
	go test ./...

test-integration: ## Run integration tests
	go test -tags=integration ./...

lint: ## Run vet, format check, and linter
	go vet ./...
	@test -z "$$(gofmt -l .)" || (echo "gofmt: unformatted files:" && gofmt -l . && exit 1)
	go tool -modfile=golangci-lint.mod golangci-lint run ./...

fmt: ## Format code
	gofmt -w .

check: lint test test-integration ## Run all checks

clean: ## Remove built binary
	rm -f jip
