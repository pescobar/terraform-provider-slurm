PROVIDER_NAME = slurm
BINARY_NAME = terraform-provider-$(PROVIDER_NAME)
VERSION = 0.1.0
OS_ARCH = $(shell go env GOOS)_$(shell go env GOARCH)

# Pinned tfplugindocs version. Bump deliberately and re-run `make docs` —
# template-rendering changes can shift output formatting in docs/.
TFPLUGINDOCS_VERSION = v0.21.0

# Local install path for OpenTofu/Terraform dev overrides
INSTALL_DIR = ~/.terraform.d/plugins/registry.terraform.io/pescobar/$(PROVIDER_NAME)/$(VERSION)/$(OS_ARCH)

.PHONY: build install clean test testacc fmt vet docs docs-check

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

# Regenerate registry documentation under docs/ from the provider schema
# and the templates under templates/. Driven by tfplugindocs at the version
# pinned above so generated output is reproducible across machines.
#
# Requirements:
#   - `terraform` (not `tofu`) on PATH. tfplugindocs invokes `terraform init`
#     to introspect the schema; OpenTofu binaries named `tofu` are not
#     auto-discovered, and even after symlinking, tfplugindocs v0.21 forces
#     the provider source path `registry.terraform.io/hashicorp/$(PROVIDER_NAME)`,
#     which only resolves with terraform's installation logic.
#   - Network access to fetch tfplugindocs deps and (if `terraform` binary
#     is not already cached) the terraform release archive.
docs:
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@$(TFPLUGINDOCS_VERSION) generate \
		--provider-name $(PROVIDER_NAME)

# Verify docs/ is in sync with the schema. Regenerates into a temp dir and
# diffs against the committed copy. Fails when they differ — wire into CI
# once a `terraform` binary is part of the workflow image.
docs-check:
	@tmpdir=$$(mktemp -d) && \
		go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@$(TFPLUGINDOCS_VERSION) generate \
			--provider-name $(PROVIDER_NAME) \
			--rendered-website-dir $$tmpdir && \
		diff -ruN docs/ $$tmpdir/ && \
		rm -rf $$tmpdir
