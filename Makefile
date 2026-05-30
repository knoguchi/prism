.PHONY: all build build-proxy build-mcp build-web run run-proxy run-mcp run-web dev clean test fmt lint

# Default target
all: build

# Build all binaries with embedded web UI
build: build-web build-proxy build-mcp
	@echo "Build complete. Run with: ./bin/prism --config ./configs/proxy.yaml"

# Build proxy server (with embedded web UI)
build-proxy: build-web
	@echo "Building proxy server (with embedded web UI)..."
	@go build -o bin/prism ./cmd/proxy

# Build proxy server without web (for development)
build-proxy-only:
	@echo "Building proxy server (API only, no web UI)..."
	@go build -o bin/prism ./cmd/proxy

# Build MCP server
build-mcp:
	@echo "Building MCP server..."
	@go build -o bin/prism-mcp ./cmd/mcp

# Run proxy server (production mode - uses embedded web UI)
run: build
	@echo "Starting Prism (production mode)..."
	@echo "Web UI: http://localhost:9090"
	@echo "Proxy:  localhost:8080"
	@./bin/prism --config ./configs/proxy.yaml

# Run proxy server (development mode - API only)
run-proxy: build-proxy-only
	@echo "Starting proxy server (API only)..."
	@echo "API:    http://localhost:9090"
	@echo "Proxy:  localhost:8080"
	@echo "Use 'make run-web' in another terminal for web UI"
	@./bin/prism --config ./configs/proxy.yaml

# Run proxy server with filtered logs (no TLS handshake noise)
run-quiet: build-proxy-only
	@echo "Starting proxy server (filtered logs)..."
	@./bin/prism --config ./configs/proxy.yaml 2>&1 | grep -v "Cannot handshake"

# Run MCP server
run-mcp: build-mcp
	@echo "Starting MCP server..."
	@./bin/prism-mcp --db ./data/proxy.db

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
	@echo "Development mode:"
	@echo "  Terminal 1: make run-proxy  (API on :9090, Proxy on :8080)"
	@echo "  Terminal 2: make run-web    (Web UI on :3000)"
	@echo ""
	@echo "Production mode (single binary):"
	@echo "  make run                    (Everything on :9090 + :8080)"

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
	@./bin/prism --generate-ca

# Install development dependencies
dev-deps:
	@echo "Installing development dependencies..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy
	@cd web && npm install

# Show help
help:
	@echo "Available targets:"
	@echo ""
	@echo "Production:"
	@echo "  build       - Build all (web + proxy + mcp) with embedded UI"
	@echo "  run         - Run production server (embedded web UI)"
	@echo ""
	@echo "Development:"
	@echo "  run-proxy   - Run proxy server (API only)"
	@echo "  run-web     - Run web dev server (hot reload)"
	@echo "  dev         - Show dev environment instructions"
	@echo ""
	@echo "Other:"
	@echo "  build-mcp   - Build MCP server"
	@echo "  run-mcp     - Run MCP server"
	@echo "  clean       - Remove build artifacts"
	@echo "  test        - Run tests"
	@echo "  fmt         - Format code"
	@echo "  lint        - Run linter"
	@echo "  deps        - Download dependencies"
	@echo "  help        - Show this help"