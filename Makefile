# Makefile for Sealos State Metric

# Go variables
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
GOFLAGS := -ldflags="-s -w"

# Docker variables
DOCKER_REGISTRY ?= ghcr.io
DOCKER_USERNAME ?= zijiren233
DOCKER_IMAGE ?= $(DOCKER_REGISTRY)/$(DOCKER_USERNAME)/sealos-state-metric
DOCKER_TAG ?= $(VERSION)

# Paths
BIN_DIR := bin
OUTPUT_BINARY := $(BIN_DIR)/sealos-state-metric

.PHONY: all
all: clean build

.PHONY: help
help:
	@echo "Sealos State Metric - Build and Deploy Tool"
	@echo ""
	@echo "Usage:"
	@echo "  make <target>"
	@echo ""
	@echo "Targets:"
	@echo "  build          Build the binary"
	@echo "  test           Run tests"
	@echo "  docker-build   Build Docker image"
	@echo "  docker-push    Push Docker image"
	@echo "  deploy         Deploy to Kubernetes"
	@echo "  clean          Clean build artifacts"
	@echo "  fmt            Format code"
	@echo "  lint           Run linter"

.PHONY: build
build:
	@echo "Building $(OUTPUT_BINARY)..."
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(GOFLAGS) -o $(OUTPUT_BINARY) ./cmd/sealos-state-metric
	@echo "Build complete: $(OUTPUT_BINARY)"

.PHONY: test
test:
	@echo "Running tests..."
	go test -v -race -coverprofile=coverage.out ./...
	@echo "Tests complete"

.PHONY: fmt
fmt:
	@echo "Formatting code..."
	go fmt ./...
	@echo "Format complete"

.PHONY: lint
lint:
	@echo "Running linter..."
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed"; exit 1; }
	golangci-lint run ./...
	@echo "Lint complete"

.PHONY: docker-build
docker-build:
	@echo "Building Docker image $(DOCKER_IMAGE):$(DOCKER_TAG)..."
	docker build -f deploy/docker/Dockerfile \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) \
		-t $(DOCKER_IMAGE):latest \
		.
	@echo "Docker build complete"

.PHONY: docker-push
docker-push: docker-build
	@echo "Pushing Docker image $(DOCKER_IMAGE):$(DOCKER_TAG)..."
	docker push $(DOCKER_IMAGE):$(DOCKER_TAG)
	docker push $(DOCKER_IMAGE):latest
	@echo "Docker push complete"

.PHONY: deploy
deploy:
	@echo "Deploying to Kubernetes..."
	kubectl apply -k deploy/kubernetes/
	@echo "Deploy complete"

.PHONY: undeploy
undeploy:
	@echo "Removing from Kubernetes..."
	kubectl delete -k deploy/kubernetes/ --ignore-not-found=true
	@echo "Undeploy complete"

.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BIN_DIR)
	rm -f coverage.out
	@echo "Clean complete"

.PHONY: tidy
tidy:
	@echo "Running go mod tidy..."
	go mod tidy
	@echo "Tidy complete"

.PHONY: verify
verify: fmt lint test
	@echo "Verification complete"

.PHONY: release
release: verify docker-build docker-push
	@echo "Release $(VERSION) complete"
