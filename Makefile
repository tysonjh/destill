.PHONY: all build build-legacy build-agentic clean test help

# Default target
all: build

# Build all binaries
build: build-legacy build-agentic

# Build legacy CLI (existing)
build-legacy:
	@echo "Building legacy destill CLI..."
	@go build -o bin/destill-legacy ./src/cmd/cli

# Build agentic mode binaries
build-agentic:
	@echo "Building agentic mode binaries..."
	@go build -o bin/destill ./src/cmd/destill-cli
	@go build -o bin/destill-ingest ./src/cmd/ingest-agent
	@go build -o bin/destill-analyze ./src/cmd/analyze-agent

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@rm -f destill destill-ingest destill-analyze destill-legacy
	@echo "Clean complete"

# Run tests
test:
	@echo "Running tests..."
	@go test ./src/broker ./src/store ./src/pipeline ./src/ingest ./src/analyze -v

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@go test ./src/broker ./src/store ./src/pipeline ./src/ingest ./src/analyze -coverprofile=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Build and install to system
install: build-agentic
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
	@echo "  build-legacy    - Build legacy CLI only"
	@echo "  build-agentic   - Build agentic mode binaries"
	@echo "  clean           - Remove build artifacts"
	@echo "  test            - Run all tests"
	@echo "  test-coverage   - Run tests with coverage report"
	@echo "  install         - Install binaries to system (requires sudo)"
	@echo "  help            - Show this help message"
	@echo ""
	@echo "Agentic mode binaries:"
	@echo "  bin/destill         - Main CLI with mode detection"
	@echo "  bin/destill-ingest  - Standalone ingest agent"
	@echo "  bin/destill-analyze - Standalone analyze agent"

