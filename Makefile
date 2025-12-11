.PHONY: all build build-agents clean test help

# Default target
all: build

# Build all binaries
build: build-agents

# Build all binaries (CLI and agents)
build-agents:
	@echo "Building destill binaries..."
	@go build -o bin/destill ./src/cmd/cli
	@go build -o bin/destill-ingest ./src/cmd/ingest-agent
	@go build -o bin/destill-analyze ./src/cmd/analyze-agent

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@rm -f destill destill-ingest destill-analyze
	@echo "Clean complete"

# Run tests
test:
	@echo "Running tests..."
	@go test ./src/broker ./src/store ./src/ingest ./src/analyze -v

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@go test ./src/broker ./src/store ./src/ingest ./src/analyze -coverprofile=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Build and install to system
install: build
	@echo "Installing binaries to /usr/local/bin..."
	@sudo cp bin/destill /usr/local/bin/
	@sudo cp bin/destill-ingest /usr/local/bin/
	@sudo cp bin/destill-analyze /usr/local/bin/
	@echo "Install complete"

# Help target
help:
	@echo "Destill Makefile"
	@echo ""
	@echo "Available targets:"
	@echo "  all             - Build all binaries (default)"
	@echo "  build           - Build all binaries"
	@echo "  build-agents    - Build all binaries"
	@echo "  clean           - Remove build artifacts"
	@echo "  test            - Run all tests"
	@echo "  test-coverage   - Run tests with coverage report"
	@echo "  install         - Install binaries to system (requires sudo)"
	@echo "  help            - Show this help message"
	@echo ""
	@echo "Binaries:"
	@echo "  bin/destill         - Main CLI (analyze: local mode, submit/view: distributed mode)"
	@echo "  bin/destill-ingest  - Standalone ingest agent"
	@echo "  bin/destill-analyze - Standalone analyze agent"

