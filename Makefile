.PHONY: help build test run clean docker-build docker-run docker-stop docker-clean

# Default target
.DEFAULT_GOAL := help

# Binary name
BINARY_NAME=gospidertrap
DOCKER_IMAGE=gospidertrap
DOCKER_TAG=latest

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the Go binary
	@echo "Building $(BINARY_NAME)..."
	@go build -o $(BINARY_NAME) .
	@echo "✓ Build complete: $(BINARY_NAME)"

build-linux: ## Build the Go binary for Linux
	@echo "Building $(BINARY_NAME) for Linux..."
	@GOOS=linux GOARCH=amd64 go build -o $(BINARY_NAME) .
	@echo "✓ Build complete: $(BINARY_NAME)"

test: ## Run tests
	@echo "Running tests..."
	@go test ./... -v
	@echo "✓ Tests passed"

test-race: ## Run tests with race detector
	@echo "Running tests with race detector..."
	@go test ./... -race
	@echo "✓ Tests passed (race detector)"

test-coverage: ## Run tests with coverage
	@echo "Running tests with coverage..."
	@go test ./... -coverprofile=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo "✓ Coverage report: coverage.html"

run: build ## Build and run the application
	@echo "Starting $(BINARY_NAME)..."
	@./$(BINARY_NAME)

clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -f $(BINARY_NAME)
	@rm -f coverage.out coverage.html
	@rm -rf data/
	@echo "✓ Cleaned"

# Docker targets
docker-build: ## Build Docker image
	@echo "Building Docker image $(DOCKER_IMAGE):$(DOCKER_TAG)..."
	@docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .
	@echo "✓ Docker image built: $(DOCKER_IMAGE):$(DOCKER_TAG)"

docker-run: ## Run Docker container
	@echo "Starting Docker container..."
	@docker run -d \
		--name $(DOCKER_IMAGE) \
		-p 8000:8000 \
		-v $$(pwd)/data:/app/data \
		$(DOCKER_IMAGE):$(DOCKER_TAG)
	@echo "✓ Container started"
	@echo ""
	@echo "View logs with: docker logs -f $(DOCKER_IMAGE)"
	@echo "Get admin URL with: docker logs $(DOCKER_IMAGE) | grep 'Login URL'"

docker-logs: ## View Docker container logs
	@docker logs -f $(DOCKER_IMAGE)

docker-stop: ## Stop Docker container
	@echo "Stopping Docker container..."
	@docker stop $(DOCKER_IMAGE) || true
	@docker rm $(DOCKER_IMAGE) || true
	@echo "✓ Container stopped"

docker-clean: docker-stop ## Stop container and remove image
	@echo "Removing Docker image..."
	@docker rmi $(DOCKER_IMAGE):$(DOCKER_TAG) || true
	@echo "✓ Docker image removed"

docker-shell: ## Open a shell in the running container
	@docker exec -it $(DOCKER_IMAGE) /bin/sh

# Docker Compose targets
compose-up: ## Start services with docker-compose
	@echo "Starting services with docker-compose..."
	@docker-compose up -d
	@echo "✓ Services started"
	@docker-compose logs --tail=20

compose-down: ## Stop services with docker-compose
	@echo "Stopping services with docker-compose..."
	@docker-compose down
	@echo "✓ Services stopped"

compose-logs: ## View docker-compose logs
	@docker-compose logs -f

compose-restart: compose-down compose-up ## Restart docker-compose services

# Development targets
dev: ## Run in development mode (auto-reload on code changes)
	@echo "Starting in development mode..."
	@which air > /dev/null || go install github.com/air-verse/air@latest
	@air

lint: ## Run linters
	@echo "Running linters..."
	@go vet ./...
	@which golangci-lint > /dev/null && golangci-lint run || echo "golangci-lint not installed, skipping"
	@echo "✓ Linting complete"

fmt: ## Format code
	@echo "Formatting code..."
	@go fmt ./...
	@echo "✓ Code formatted"

mod-tidy: ## Tidy go modules
	@echo "Tidying go modules..."
	@go mod tidy
	@echo "✓ Modules tidied"

mod-verify: ## Verify go modules
	@echo "Verifying go modules..."
	@go mod verify
	@echo "✓ Modules verified"

all: clean lint test build ## Run all checks and build
	@echo "✓ All checks passed"
