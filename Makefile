# PromptArena Makefile
#
# PromptArena is a single Go module (github.com/AltairaLabs/promptarena) that
# ships two binaries — promptarena and packc — plus a pnpm-managed React
# frontend (arena/web/frontend) that is built and embedded into the binary.
#
# Targets mirror the CI pipeline (.github/workflows/ci.yml), so `make verify`
# reproduces the merge gates locally before you push. Run `make help` for the
# list.
#
# Note: no C toolchain is needed. Audio (oto speaker playback and PortAudio for
# the voice console) is loaded at runtime via purego, so CGO_ENABLED=0 builds
# keep it. The frontend pulls @altairalabs/atlas from GitHub Packages (private), so
# `make deps` needs npm auth configured for read:packages.

# Recipes use bash features (process substitution, arithmetic); pin the shell so
# they don't fall back to /bin/sh (dash on CI).
SHELL        := bash

GO           ?= go
WEB          := arena/web/frontend
PNPM         := pnpm --dir $(WEB)
COVERAGE_OUT := coverage.out
# Base revision the "new code" lint gate diffs against (matches CI's
# only-new-issues). Override for a different base: `make lint-new LINT_BASE=…`.
LINT_BASE    ?= origin/main

.DEFAULT_GOAL := help
.PHONY: help deps build build-go build-web run test test-web test-race coverage \
        test-examples lint lint-new check-weak-assertions vet fmt schemas schemas-check schemas-format \
        verify hooks dev-web clean release-snapshot

# Route unknown targets to help rather than a bare "No rule to make target".
.DEFAULT:
	@$(MAKE) help

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# ── Dependencies ────────────────────────────────────────────────────────────

deps: ## Download Go modules and install frontend deps (needs GH Packages auth)
	@echo "Downloading Go modules…"
	@$(GO) mod download
	@echo "Installing frontend deps…"
	@$(PNPM) install

# ── Build ───────────────────────────────────────────────────────────────────

# The React frontend is compiled to arena/web/frontend/dist and embedded into
# the binary via //go:embed (arena/web/embed.go). So a binary only reflects UI
# changes if the frontend was rebuilt FIRST — hence `build` does the web build
# before the Go build. Use `build-go` for the fast path when the UI is unchanged.
build: build-web build-go ## Build the frontend + binaries with the CURRENT UI embedded

build-go: ## Build only the Go binaries into bin/ (embeds whatever dist/ holds now)
	@echo "Building promptarena…"
	@$(GO) build -o bin/promptarena ./arena/cmd/promptarena
	@echo "Building packc…"
	@$(GO) build -o bin/packc ./packc
	@echo "✓ Binaries in bin/"

build-web: ## Build the React frontend (tsc + vite) that embeds into the binary
	@echo "Building frontend…"
	@$(PNPM) build

run: build ## Build (incl. frontend), then run promptarena (usage: make run ARGS="…")
	@./bin/promptarena $(ARGS)

# ── Test ────────────────────────────────────────────────────────────────────

test: ## Run the full Go test suite
	@$(GO) test ./... -count=1

test-web: ## Run the frontend test suite (vitest)
	@$(PNPM) test

test-race: ## Run the Go test suite with the race detector
	@$(GO) test -race ./... -count=1

coverage: ## Run Go tests with coverage and print the total (writes coverage.out)
	@$(GO) test ./... -coverprofile=$(COVERAGE_OUT) -count=1
	@$(GO) tool cover -func=$(COVERAGE_OUT) | grep '^total:' || true

test-examples: build-go ## Validate every example config and run every one that needs no API key
	@scripts/test-examples.sh

# ── Lint & format ───────────────────────────────────────────────────────────

lint: ## Run golangci-lint over all code (includes inherited/grandfathered debt)
	@golangci-lint run ./...

lint-new: ## Lint only new code vs $(LINT_BASE) — matches the CI "new code" gate
	@golangci-lint run --new-from-rev=$(LINT_BASE) --timeout=5m ./...

check-weak-assertions: ## Fail on new tests whose assertions cannot fail (CI "Weak Assertion Guard")
	@scripts/check-weak-assertions.sh

vet: ## Run go vet
	@$(GO) vet ./...

fmt: ## Format Go code with gofmt
	@gofmt -w .

# ── Schemas ─────────────────────────────────────────────────────────────────

schemas: ## Regenerate the JSON schemas, and the copies embedded in the binary
	@$(GO) run ./tools/schema-gen/
	@$(GO) generate ./arena/agentkb/schema.go

schemas-check: ## Fail if committed schemas are out of date (CI guard)
	@$(GO) run ./tools/schema-gen/ --check

schemas-format: ## Reformat the committed schemas without regenerating them
	@$(GO) run ./tools/schema-gen/ --format

# ── Docs ────────────────────────────────────────────────────────────────────

docs-voice: ## Check docs prose against docs/STYLE.md (fails only in enforced dirs)
	@bash scripts/check-docs-voice.sh

docs-stale: ## List docs pages whose verified sources changed (warn-only)
	@bash scripts/docs-stale.sh

docs-coverage: ## List public-surface files no docs page cites (warn-only)
	@bash scripts/docs-coverage.sh

docs-scripts-test: ## Test the docs-review scripts (voice, stale, provenance, coverage)
	@for t in scripts/check-docs-voice_test.sh scripts/docs-stale_test.sh \
		scripts/docs-provenance_test.sh scripts/docs-coverage_test.sh; do bash $$t || exit 1; done

# ── Meta ────────────────────────────────────────────────────────────────────

verify: ## Reproduce the CI gates locally: build, test+coverage, schemas, lint, frontend
	@echo "▶ go build ./..."      && $(GO) build ./...
	@echo "▶ go test (coverage)"  && $(MAKE) --no-print-directory coverage
	@echo "▶ schemas up to date"  && $(MAKE) --no-print-directory schemas-check
	@echo "▶ lint (new code)"     && $(MAKE) --no-print-directory lint-new
	@echo "▶ frontend build"      && $(MAKE) --no-print-directory build-web
	@echo "▶ frontend tests"      && $(MAKE) --no-print-directory test-web
	@echo "✓ verify passed"

hooks: ## Install the git pre-commit and commit-msg (DCO) hooks
	@./scripts/install-hooks.sh

dev-web: ## Start the frontend dev server (vite)
	@$(PNPM) dev

clean: ## Remove build artifacts and coverage reports
	@rm -rf bin/
	@rm -f $(COVERAGE_OUT) coverage.html
	@rm -rf $(WEB)/dist $(WEB)/coverage $(WEB)/junit.xml
	@echo "✓ Cleaned build artifacts"

release-snapshot: ## Build a local release snapshot with goreleaser (no publish)
	@goreleaser release --snapshot --clean
