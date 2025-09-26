# Local Chat Application Makefile

APP_NAME=out/ratata

.PHONY: build run clean test deps fmt lint help

# Default target
all: build

# Build the application
build:
	@echo "Building localchat..."
	go build -o $(APP_NAME) cmd/main.go

# Run the application (interactive username prompt)
run: build
	./$(APP_NAME)

# Run with CLI flags
run-cli: build
	./$(APP_NAME) -username alice -room dev

# Run with debug logging
run-debug:
	LOG_LEVEL=DEBUG ./$(APP_NAME)

# Run with custom username via CLI
run-user: build
	./$(APP_NAME) -username $(USER) -room general

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f $(APP_NAME)

# Install dependencies
deps:
	@echo "Installing dependencies..."
	go mod tidy
	go mod download

# Run tests
test:
	@echo "Running tests..."
	go test ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -cover ./...

# Run tests with race detection
test-race:
	@echo "Running tests with race detection..."
	go test -race ./...

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Run linter
lint:
	@echo "Running linter..."
	golangci-lint run

# Build for multiple platforms
build-all: build
	@echo "Building for multiple platforms..."
	GOOS=linux GOARCH=amd64 go build -o $(APP_NAME)-linux-amd64 cmd/main.go
	GOOS=darwin GOARCH=amd64 go build -o $(APP_NAME)-darwin-amd64 cmd/main.go
	GOOS=darwin GOARCH=arm64 go build -o $(APP_NAME)-darwin-arm64 cmd/main.go
	GOOS=windows GOARCH=amd64 go build -o $(APP_NAME)-windows-amd64.exe cmd/main.go

# Install tools for development
install-tools:
	@echo "Installing development tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Show help
help:
	@echo "Available targets:"
	@echo "  build        - Build the application"
	@echo "  run          - Build and run the application"
	@echo "  run-debug    - Run with debug logging"
	@echo "  clean        - Clean build artifacts"
	@echo "  deps         - Install dependencies"
	@echo "  test         - Run tests"
	@echo "  test-coverage- Run tests with coverage"
	@echo "  test-race    - Run tests with race detection"
	@echo "  fmt          - Format code"
	@echo "  lint         - Run linter"
	@echo "  build-all    - Build for multiple platforms"
	@echo "  install-tools - Install development tools"
	@echo "  help         - Show this help"
