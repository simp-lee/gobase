.PHONY: help build run dev test lint clean download-vendor

## Default target: show help
help: ## Show available commands
	@echo ""
	@echo  Available commands:
	@echo  -------------------
	@echo  make build            - Build the server binary to bin/server
	@echo  make run              - Run the server (default config)
	@echo  make dev              - Run the server with configs/config.yaml
	@echo  make test             - Run all tests with verbose output
	@echo  make lint             - Run golangci-lint
	@echo  make clean            - Remove build artifacts
	@echo  make download-vendor  - Download frontend vendor assets (htmx, Alpine.js, Tailwind CSS)
	@echo ""

## Build & Run
build: ## Build the server binary
	mkdir -p bin
	go build -o bin/server ./cmd/server

run: ## Run the server
	go run ./cmd/server

dev: ## Run the server with dev config
	go run ./cmd/server -config configs/config.yaml

## Quality
test: ## Run all tests
	go test ./... -v

lint: ## Run golangci-lint
	golangci-lint run ./...

## Cleanup
# NOTE: make targets require a Unix-like shell (bash, WSL, or Git Bash on Windows).
clean: ## Remove build artifacts
	rm -rf bin

## Vendor assets
download-vendor: ## Download frontend vendor assets
	mkdir -p web/static/vendor
	curl -sfL -o web/static/vendor/htmx.min.js https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js
	curl -sfL -o web/static/vendor/alpine.min.js https://unpkg.com/alpinejs@3.14.8/dist/cdn.min.js
	curl -sfL -o web/static/vendor/tailwind.css https://cdn.tailwindcss.com/4
	@echo Vendor assets downloaded to web/static/vendor/
