# Variables
AGENT_VERSION := v2.20.3
IMMUCORE_VERSION := v0.9.4
KCRYPT_VERSION := v0.15.0
KCRYPT_CHALLENGER_VERSION := v0.11.1
ARCH := $(shell uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')
BINARY_NAMES := kairos-agent immucore kcrypt kcrypt-discovery-challenger
OUTPUT_DIR := pkg/bundled/binaries

# URLs for binaries
kairos-agent_URL := https://github.com/kairos-io/kairos-agent/releases/download/$(AGENT_VERSION)/kairos-agent-$(AGENT_VERSION)-Linux-$(ARCH).tar.gz
immucore_URL := https://github.com/kairos-io/immucore/releases/download/$(IMMUCORE_VERSION)/immucore-$(IMMUCORE_VERSION)-Linux-$(ARCH).tar.gz
kcrypt_URL := https://github.com/kairos-io/kcrypt/releases/download/$(KCRYPT_VERSION)/kcrypt-$(KCRYPT_VERSION)-Linux-$(ARCH).tar.gz
kcrypt-discovery-challenger_URL := https://github.com/kairos-io/kcrypt-challenger/releases/download/$(KCRYPT_CHALLENGER_VERSION)/kcrypt-discovery-challenger-$(KCRYPT_CHALLENGER_VERSION)-Linux-$(ARCH).tar.gz

.PHONY: all prepare download compress cleanup

all: prepare download compress cleanup

# Clean the output directory
prepare:
	@echo "Cleaning up the output directory..."
	@rm -rf $(OUTPUT_DIR)
	@if ! command -v upx >/dev/null 2>&1; then \
	  echo "Error: upx binary is not available. Please install upx."; \
	  exit 1; \
	fi

# Ensure the bundled directory exists
$(OUTPUT_DIR):
	@echo "Creating directory $(OUTPUT_DIR)..."
	@mkdir -p $(OUTPUT_DIR)

# Download each binary
$(OUTPUT_DIR)/%:
	@echo "Downloading and extracting $* for architecture $(ARCH)..."
	@mkdir -p $(OUTPUT_DIR)
	@curl -L -s $($*_URL) | tar -xz -C $(OUTPUT_DIR)

# Download all binaries
download: $(addprefix $(OUTPUT_DIR)/, $(BINARY_NAMES))

# Run upx to compress binaries unless SKIP_UPX is set
compress:
	@if [ -z "$(SKIP_UPX)" ]; then \
		echo "Running upx compress..."; \
		upx -q -1 $(addprefix $(OUTPUT_DIR)/, $(BINARY_NAMES)); \
	else \
		echo "Skipping upx compression as SKIP_UPX is set"; \
	fi
# Remove non-binary files from the output directory
cleanup:
	@echo "Cleaning up non-binary files..."
	@find $(OUTPUT_DIR) -type f ! -exec file {} \; | grep -v "executable" | awk -F: '{print $$1}' | xargs -r rm -f