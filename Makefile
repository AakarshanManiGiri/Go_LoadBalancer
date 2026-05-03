.PHONY: help build run test clean demo lint fmt

# Default target
help:
	@echo "Go HTTP Load Balancer - Makefile Commands"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  build          Build the load balancer binary"
	@echo "  run            Run the load balancer with config.yaml"
	@echo "  test           Run all tests"
	@echo "  test-verbose   Run tests with verbose output"
	@echo "  test-race      Run tests with race detector"
	@echo "  test-coverage  Run tests with coverage report"
	@echo "  demo           Run demo with mock backends"
	@echo "  lint           Run linter (requires golangci-lint)"
	@echo "  fmt            Format code"
	@echo "  vet            Run go vet"
	@echo "  clean          Remove build artifacts"
	@echo "  deps           Download dependencies"
	@echo ""

# Build targets
build:
	@echo "Building load balancer..."
	go build -o loadbalancer -v
	@echo "✓ Build complete"

run: build
	@echo "Starting load balancer..."
	./loadbalancer -config config.yaml

# Test targets
test:
	@echo "Running tests..."
	go test -v ./...
	@echo "✓ Tests passed"

test-verbose:
	@echo "Running tests (verbose)..."
	go test -v -race ./...

test-race:
	@echo "Running tests with race detector..."
	go test -race ./...

test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "✓ Coverage report: coverage.html"

# Demo target
demo: build
	@echo "Running demo with mock backends..."
	bash demo.sh

# Code quality targets
lint:
	@if ! command -v golangci-lint &> /dev/null; then \
		echo "Installing golangci-lint..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	fi
	golangci-lint run ./...

fmt:
	@echo "Formatting code..."
	gofmt -s -w .
	go mod tidy
	@echo "✓ Code formatted"

vet:
	@echo "Running go vet..."
	go vet ./...
	@echo "✓ Vet passed"

# Dependency targets
deps:
	@echo "Downloading dependencies..."
	go mod download
	go mod tidy
	@echo "✓ Dependencies downloaded"

# Clean target
clean:
	@echo "Cleaning up..."
	rm -f loadbalancer
	rm -f coverage.out coverage.html
	go clean
	@echo "✓ Clean complete"

# Docker targets (optional)
docker-build:
	@echo "Building Docker image..."
	docker build -t go-loadbalancer .

docker-run:
	@echo "Running Docker container..."
	docker run -p 8080:8080 -v $(PWD)/config.yaml:/app/config.yaml go-loadbalancer

# Installation target
install: build
	@echo "Installing load balancer..."
	@mkdir -p $(HOME)/.local/bin
	@cp loadbalancer $(HOME)/.local/bin/
	@echo "✓ Installed to $(HOME)/.local/bin/loadbalancer"
	@echo "Add $(HOME)/.local/bin to your PATH to use 'loadbalancer' from anywhere"
