.PHONY: build build-all clean test release help

# Default target
help:
	@echo "Available targets:"
	@echo "  build       - Build for current platform"
	@echo "  build-all   - Build for all platforms"
	@echo "  clean       - Remove build artifacts"
	@echo "  test        - Run tests"
	@echo "  release     - Create a new release (requires version argument)"
	@echo ""
	@echo "Examples:"
	@echo "  make build"
	@echo "  make build-all"
	@echo "  make release VERSION=1.0.0"

# Build for current platform
build:
	go build -ldflags="-s -w" -o tendrl-agent .

# Build for all platforms
build-all: clean
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o tendrl-agent-linux-amd64 .
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o tendrl-agent-linux-arm64 .
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o tendrl-agent-windows-amd64.exe .
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o tendrl-agent-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o tendrl-agent-darwin-arm64 .

# Clean build artifacts
clean:
	rm -f tendrl-agent tendrl-agent-*

# Run tests
test:
	go test -v ./...

# Create a new release
release:
	@if [ -z "$(VERSION)" ]; then \
		echo "Error: VERSION is required. Usage: make release VERSION=1.0.0"; \
		exit 1; \
	fi
	@echo "Creating release v$(VERSION)..."
	@./scripts/release.sh $(VERSION) 