.PHONY: all build build-proxy build-mcp build-web run run-proxy run-mcp run-web dev clean test fmt lint

# Default target
all: build

# Build all binaries and web
build: build-proxy build-mcp build-web

# Build proxy server
build-proxy:
	@echo "Building proxy server..."
	@go build -o bin/ai-proxy ./cmd/proxy

# Build MCP server
build-mcp:
	@echo "Building MCP server..."
	@go build -o bin/ai-proxy-mcp ./cmd/mcp

# Run proxy server
run: run-proxy

run-proxy: build-proxy
	@echo "Starting proxy server..."
	@./bin/ai-proxy --config ./configs/proxy.yaml

# Run proxy server with filtered logs (no TLS handshake noise)
run-quiet: build-proxy
	@echo "Starting proxy server (filtered logs)..."
	@./bin/ai-proxy --config ./configs/proxy.yaml 2>&1 | grep -v "Cannot handshake"

# Run MCP server
run-mcp: build-mcp
	@echo "Starting MCP server..."
	@./bin/ai-proxy-mcp --db ./data/proxy.db

# Build web UI
build-web:
	@echo "Building web UI..."
	@cd web && npm install && npm run build

# Run web dev server
run-web:
	@echo "Starting web dev server..."
	@cd web && npm run dev

# Run full dev environment (proxy + web)
dev:
	@echo "Starting development environment..."
	@echo "Run these in separate terminals:"
	@echo "  make run-proxy  (proxy on :8080, API on :9090)"
	@echo "  make run-web    (web UI on :3000)"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf bin/
	@rm -rf data/proxy.db
	@rm -rf web/dist
	@rm -rf web/node_modules

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Run linter
lint:
	@echo "Running linter..."
	@golangci-lint run

# Generate CA certificate
generate-ca:
	@echo "Generating CA certificate..."
	@mkdir -p configs/ca
	@./bin/ai-proxy --generate-ca

# Install development dependencies
dev-deps:
	@echo "Installing development dependencies..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy

# Show help
help:
	@echo "Available targets:"
	@echo "  build       - Build all (proxy, mcp, web)"
	@echo "  build-proxy - Build proxy server"
	@echo "  build-mcp   - Build MCP server"
	@echo "  build-web   - Build web UI"
	@echo "  run         - Run proxy server"
	@echo "  run-mcp     - Run MCP server"
	@echo "  run-web     - Run web dev server"
	@echo "  dev         - Show dev environment instructions"
	@echo "  clean       - Remove build artifacts"
	@echo "  test        - Run tests"
	@echo "  fmt         - Format code"
	@echo "  lint        - Run linter"
	@echo "  deps        - Download dependencies"
	@echo "  help        - Show this help"
