.PHONY: build clean gen-proto nix-build nix-shell nix-docker nix-docker-load nix-run vendor vendor-all docker-build docker-run test run

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=lake-writer

# Nix parameters
NIX=nix
NIX_BUILD=$(NIX) build
NIX_DEVELOP=$(NIX) develop
NIX_RUN=$(NIX) run

# Docker parameters
DOCKER_IMAGE=obsrvr-lake-writer
DOCKER_TAG=latest

# Proto parameters
PROTOC=protoc
GO_SRC_DIR=./go
GEN_DIR=$(GO_SRC_DIR)/gen
PROTO_DIR=./protos
GO_PACKAGE_BASE=github.com/withObsrvr/obsrvr-lake-writer

all: build

# Build the server
build: gen-proto
	@echo "Building $(BINARY_NAME)..."
	cd $(GO_SRC_DIR) && GOWORK=off $(GOBUILD) -o ../$(BINARY_NAME) ./cmd/server/main.go
	@echo "✓ Build completed: $(BINARY_NAME)"

# Build with vendored dependencies (offline mode)
build-offline:
	@echo "Building $(BINARY_NAME) in offline mode..."
	cd $(GO_SRC_DIR) && GOWORK=off GO111MODULE=on GOPROXY=off go build -mod=vendor -o ../$(BINARY_NAME) ./cmd/server/main.go
	@echo "✓ Build completed in offline mode: $(BINARY_NAME)"

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -rf $(GEN_DIR)
	rm -rf result

# Initialize build environment
init:
	@echo "Initializing build environment..."
	@rm -rf $(GEN_DIR) > /dev/null 2>&1 || true
	@mkdir -p $(GEN_DIR) > /dev/null 2>&1 || true
	@echo "✓ Init completed for lake-writer"

# Generate protobuf code
gen-proto: init
	@echo "Generating protobuf code..."
	@protoc \
		--proto_path=$(PROTO_DIR) \
		--go_out=$(GEN_DIR) \
		--go_opt=paths=source_relative \
		--go_opt=Mlake_writer/lake_writer.proto=$(GO_PACKAGE_BASE)/gen/lake_writer \
		--go-grpc_out=$(GEN_DIR) \
		--go-grpc_opt=paths=source_relative \
		--go-grpc_opt=Mlake_writer/lake_writer.proto=$(GO_PACKAGE_BASE)/gen/lake_writer \
		lake_writer/lake_writer.proto
	@echo "Creating go.mod for generated code..."
	@echo 'module $(GO_PACKAGE_BASE)/gen/lake_writer' > $(GEN_DIR)/lake_writer/go.mod
	@echo '' >> $(GEN_DIR)/lake_writer/go.mod
	@echo 'go 1.25' >> $(GEN_DIR)/lake_writer/go.mod
	@echo '' >> $(GEN_DIR)/lake_writer/go.mod
	@echo 'require (' >> $(GEN_DIR)/lake_writer/go.mod
	@echo '	google.golang.org/grpc v1.67.1' >> $(GEN_DIR)/lake_writer/go.mod
	@echo '	google.golang.org/protobuf v1.35.2' >> $(GEN_DIR)/lake_writer/go.mod
	@echo ')' >> $(GEN_DIR)/lake_writer/go.mod
	@grep -q "replace github.com/withObsrvr/obsrvr-lake-writer/gen/lake_writer" $(GO_SRC_DIR)/go.mod || echo 'replace github.com/withObsrvr/obsrvr-lake-writer/gen/lake_writer => ./gen/lake_writer' >> $(GO_SRC_DIR)/go.mod
	@cd $(GO_SRC_DIR) && GOWORK=off go mod tidy
	@echo "✓ Proto generation completed"

# Run the server
run: build
	./$(BINARY_NAME) -config config/local.yaml

# Run tests
test:
	cd $(GO_SRC_DIR) && GOWORK=off $(GOTEST) -v ./...

# Vendor dependencies for offline builds
vendor:
	cd $(GO_SRC_DIR) && GOWORK=off $(GOCMD) mod tidy && GOWORK=off $(GOCMD) mod vendor
	@echo "✓ Dependencies vendored for offline builds"

# Generate proto and vendor dependencies (all-in-one)
vendor-all: gen-proto vendor

# --- Nix-based build targets ---

# Build with Nix
nix-build:
	$(NIX_BUILD)
	@echo "✓ Nix build completed. Binary is available at ./result/bin/$(BINARY_NAME)"

# Build Docker image with Nix
nix-docker:
	$(NIX_BUILD) .#docker
	@echo "✓ Docker image built with Nix. Load with: make nix-docker-load"

# Load Nix-built Docker image
nix-docker-load:
	docker load < result
	@echo "✓ Docker image loaded: $(DOCKER_IMAGE):$(DOCKER_TAG)"

# Run with Nix
nix-run:
	$(NIX_RUN)

# Enter Nix development shell
nix-shell:
	$(NIX_DEVELOP)

# --- Docker targets ---

# Build Docker image from Nix-built binary
docker-build: nix-build
	cp ./result/bin/$(BINARY_NAME) ./$(BINARY_NAME)
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .
	rm -f ./$(BINARY_NAME)
	@echo "✓ Docker image built: $(DOCKER_IMAGE):$(DOCKER_TAG)"

# Run Docker container
docker-run:
	docker run -p 50099:50099 -p 8088:8088 \
		-e AWS_ACCESS_KEY_ID=${AWS_ACCESS_KEY_ID} \
		-e AWS_SECRET_ACCESS_KEY=${AWS_SECRET_ACCESS_KEY} \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

# --- CI/CD shortcuts ---

# Complete build process (for CI)
ci-build: nix-build nix-docker

# Release process (build and load Docker image)
release: nix-build docker-build
