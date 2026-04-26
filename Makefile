PROVIDER_NAME = slurm
BINARY_NAME = terraform-provider-$(PROVIDER_NAME)
VERSION = 0.1.0
OS_ARCH = $(shell go env GOOS)_$(shell go env GOARCH)

# Local install path for OpenTofu/Terraform dev overrides
INSTALL_DIR = ~/.terraform.d/plugins/registry.terraform.io/pescobar/$(PROVIDER_NAME)/$(VERSION)/$(OS_ARCH)

.PHONY: build install clean test testacc fmt vet

# Build the provider binary
build:
	go build -ldflags="-X main.version=$(VERSION)" -o $(BINARY_NAME)

# Install the provider locally for development testing
install: build
	mkdir -p $(INSTALL_DIR)
	cp $(BINARY_NAME) $(INSTALL_DIR)/

# Run unit tests
test:
	go test ./... -v

# Run acceptance tests (requires a running Slurm test environment)
testacc:
	TF_ACC=1 go test ./... -v -timeout 120m

# Format code
fmt:
	go fmt ./...

# Run go vet
vet:
	go vet ./...

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -rf $(INSTALL_DIR)
