.PHONY: help build run dev test lint clean migrate untrack-config download-vendor verify-vendor seed

HTMX_VERSION := 2.0.4
ALPINE_VERSION := 3.14.8
TAILWIND_BROWSER_VERSION := 4.2.0

HTMX_SHA256 := e209dda5c8235479f3166defc7750e1dbcd5a5c1808b7792fc2e6733768fb447
ALPINE_SHA256 := b600e363d99d95444db54acbfb2deffec9ae792aa99a09229bcda078e5b55643
TAILWIND_SHA256 := 21ea224c2ebdef220843aae27d535f4c3e632abcc03ab03730801c7cd3e4ea0e

## Default target: show help
help: ## Show available commands
	@echo ""
	@echo "  Available commands:"
	@echo "  -------------------"
	@echo "  make build            - Build the server binary to bin/server"
	@echo "  make run              - Run the server (default config)"
	@echo "  make dev              - Run the server with configs/config.yaml"
	@echo "  make test             - Run all tests with verbose output"
	@echo "  make lint             - Run golangci-lint"
	@echo "  make clean            - Remove build artifacts"
	@echo "  make seed             - Seed the database with sample data"
	@echo "  make migrate          - Run database migrations and exit"
	@echo "  make verify-vendor    - Verify required frontend vendor assets exist"
	@echo "  make untrack-config   - Remove configs/config.yaml from git tracking (keeps local file)"
	@echo "  make download-vendor  - Download frontend vendor assets (htmx, Alpine.js, Tailwind v4)"
	@echo ""

## Build & Run
build: verify-vendor ## Build the server binary
	mkdir -p bin
	go build -o bin/server ./cmd/server

run: verify-vendor ## Run the server
	go run ./cmd/server

dev: verify-vendor ## Run the server with dev config
	go run ./cmd/server -config configs/config.yaml

## Database
migrate: ## Run database migrations and exit
	go run ./cmd/server -config configs/config.yaml -migrate-only

## Git helpers
# After adding configs/config.yaml to .gitignore, run this once to stop tracking
# the file. It removes the file from git's index (so future commits ignore it)
# but keeps your local copy intact.
untrack-config: ## Remove configs/config.yaml from git tracking (keeps local file)
	git rm --cached -f configs/config.yaml || true
	@echo "configs/config.yaml is now untracked. Commit this change."

## Quality
test: ## Run all tests
	go test -race ./... -v

lint: ## Run golangci-lint
	golangci-lint run ./...

## Cleanup
# NOTE: make targets require a Unix-like shell (bash, WSL, or Git Bash on Windows).
clean: ## Remove build artifacts
	rm -rf bin

## Data
seed: ## Seed database with sample data
	go run ./cmd/seed -config configs/config.yaml

## Vendor assets
verify-vendor: ## Verify required frontend vendor assets exist
	@test -f web/static/vendor/htmx.min.js || (echo "Missing web/static/vendor/htmx.min.js. Run 'make download-vendor'." >&2; exit 1)
	@test -f web/static/vendor/alpine.min.js || (echo "Missing web/static/vendor/alpine.min.js. Run 'make download-vendor'." >&2; exit 1)
	@test -f web/static/vendor/tailwind.js || (echo "Missing web/static/vendor/tailwind.js. Run 'make download-vendor'." >&2; exit 1)

download-vendor: ## Download frontend vendor assets
	mkdir -p web/static/vendor
	tmp_dir=$$(mktemp -d) && \
	trap 'rm -rf "$$tmp_dir"' EXIT && \
	curl -sfL -o "$$tmp_dir/htmx.min.js" https://unpkg.com/htmx.org@$(HTMX_VERSION)/dist/htmx.min.js && \
	echo "$(HTMX_SHA256)  $$tmp_dir/htmx.min.js" | sha256sum -c - && \
	mv "$$tmp_dir/htmx.min.js" web/static/vendor/htmx.min.js && \
	curl -sfL -o "$$tmp_dir/alpine.min.js" https://unpkg.com/alpinejs@$(ALPINE_VERSION)/dist/cdn.min.js && \
	echo "$(ALPINE_SHA256)  $$tmp_dir/alpine.min.js" | sha256sum -c - && \
	mv "$$tmp_dir/alpine.min.js" web/static/vendor/alpine.min.js && \
	curl -sfL -o "$$tmp_dir/tailwind.js" https://unpkg.com/@tailwindcss/browser@$(TAILWIND_BROWSER_VERSION) && \
	echo "$(TAILWIND_SHA256)  $$tmp_dir/tailwind.js" | sha256sum -c - && \
	mv "$$tmp_dir/tailwind.js" web/static/vendor/tailwind.js
	@echo Vendor assets downloaded to web/static/vendor/
