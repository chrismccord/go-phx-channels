# Go Phoenix Channels - Makefile

.PHONY: help test test-unit test-integration fmt lint clean examples server deps

# Default target
help: ## Show this help message
	@echo "Go Phoenix Channels - Available Commands:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

deps: ## Install dependencies
	go mod download
	go mod tidy

fmt: ## Format Go code
	go fmt ./...

lint: ## Run linting
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found. Install it with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		go vet ./...; \
	fi

test-unit: ## Run unit tests only
	go test -v -short ./...

test-integration: ## Run integration tests (requires Phoenix server)
	@echo "Starting integration tests..."
	@echo "Make sure Phoenix test server is running: cd test_server && mix phx.server"
	go test -v ./... -run Integration

test: deps fmt ## Run all tests (unit + integration)
	@echo "Running unit tests..."
	go test -v -short ./...
	@echo ""
	@echo "Running integration tests..."
	@echo "Make sure Phoenix test server is running: cd test_server && mix phx.server"
	go test -v ./... -run Integration

test-coverage: ## Run tests with coverage
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

server: ## Start Phoenix test server
	@echo "Starting Phoenix test server on http://localhost:4000"
	cd test_server && mix deps.get && mix phx.server

server-bg: ## Start Phoenix test server in background
	@echo "Starting Phoenix test server in background..."
	cd test_server && mix deps.get && mix phx.server &
	@echo "Waiting for server to start..."
	@sleep 3

examples: ## Run all examples
	@echo "Running basic usage example..."
	cd examples/basic_usage && timeout 10s go run main.go || true
	@echo ""
	@echo "Running binary data example..."
	cd examples/binary_data && timeout 5s go run main.go || true
	@echo ""
	@echo "Examples completed. For interactive chat example, run: cd examples/chat_room && go run main.go <username>"

benchmark: ## Run benchmarks
	go test -bench=. -benchmem ./...

clean: ## Clean build artifacts and test files
	go clean
	rm -f coverage.out coverage.html
	go mod tidy

build: deps fmt lint ## Build the project
	go build ./...

install: ## Install the package
	go install ./...

docs: ## Generate documentation
	@echo "Generating Go documentation..."
	@echo "View docs at: http://localhost:6060/pkg/github.com/go-phx-channels/"
	godoc -http=:6060

# Development helpers
dev-setup: ## Set up development environment
	@echo "Setting up development environment..."
	go mod download
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "Installing golangci-lint..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	fi
	cd test_server && mix deps.get
	@echo "Development environment ready!"

dev-test: server-bg test ## Full development test (start server + run tests)
	@echo "All tests completed successfully!"

# Phoenix server management
server-deps: ## Install Phoenix server dependencies
	cd test_server && mix deps.get

server-reset: ## Reset Phoenix server (clean deps and reinstall)
	cd test_server && rm -rf _build deps && mix deps.get

# Git hooks
pre-commit: fmt lint test-unit ## Run pre-commit checks

# Release helpers
tag: ## Create a git tag (usage: make tag VERSION=v1.0.0)
	@if [ -z "$(VERSION)" ]; then \
		echo "Please provide VERSION: make tag VERSION=v1.0.0"; \
		exit 1; \
	fi
	git tag -a $(VERSION) -m "Release $(VERSION)"
	git push origin $(VERSION)

# Docker helpers (if you want to containerize tests)
docker-test: ## Run tests in Docker (if Dockerfile exists)
	@if [ -f Dockerfile ]; then \
		docker build -t go-phx-channels-test . && \
		docker run --rm go-phx-channels-test; \
	else \
		echo "Dockerfile not found. Run 'make test' instead."; \
	fi

# Check if Phoenix server is running
check-server: ## Check if Phoenix test server is running
	@if curl -s http://localhost:4000 >/dev/null 2>&1; then \
		echo "✅ Phoenix server is running"; \
	else \
		echo "❌ Phoenix server is not running. Start with: make server"; \
		exit 1; \
	fi

# Security check
security: ## Run security checks
	@if command -v gosec >/dev/null 2>&1; then \
		gosec ./...; \
	else \
		echo "gosec not found. Install it with: go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest"; \
	fi

# Dependency updates
update-deps: ## Update dependencies
	go get -u ./...
	go mod tidy
	cd test_server && mix deps.update --all

# Full quality check
quality: fmt lint test security ## Run all quality checks

# Quick development cycle
quick: fmt test-unit ## Quick development cycle (format + unit tests)

# Release preparation
release-prep: clean deps fmt lint test benchmark ## Prepare for release