.PHONY: build clean test help

# Default target
all: build

# Build the CLI
build:
	@echo "Building Aerospike Cloud CLI..."
	GOWORK=off go build -o aerospike-cloud-cli .

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -f aerospike-cloud-cli

# Run tests
test:
	@echo "Running tests..."
	go test ./...

# Install dependencies
deps:
	@echo "Installing dependencies..."
	go mod tidy
	go mod download

# Show help
help:
	@echo "Available targets:"
	@echo "  build  - Build the CLI binary"
	@echo "  clean  - Remove build artifacts"
	@echo "  test   - Run tests"
	@echo "  deps   - Install dependencies"
	@echo "  help   - Show this help message"
