# Variables
AGENT_VERSION := v2.20.4
IMMUCORE_VERSION := v0.10.0
KCRYPT_CHALLENGER_VERSION := v0.11.2
AGENT_PROVIDER_VERSION := v2.12.0
EDGEVPN_VERSION := v0.30.2
ARCH := $(shell uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')
BINARY_NAMES := kairos-agent immucore kcrypt-discovery-challenger kairos-cli
OUTPUT_DIR := pkg/bundled/binaries
OUTPUT_DIR_FIPS := pkg/bundled/binaries/fips

# URLs for binaries
define URL_TEMPLATE
https://github.com/kairos-io/$1/releases/download/$2/$3-$2-Linux-$(ARCH)$4.tar.gz
endef

kairos-agent_URL := $(call URL_TEMPLATE,kairos-agent,$(AGENT_VERSION),kairos-agent)
immucore_URL := $(call URL_TEMPLATE,immucore,$(IMMUCORE_VERSION),immucore)
kcrypt-discovery-challenger_URL := $(call URL_TEMPLATE,kcrypt-challenger,$(KCRYPT_CHALLENGER_VERSION),kcrypt-discovery-challenger)
kairos-cli_URL := $(call URL_TEMPLATE,provider-kairos,$(AGENT_PROVIDER_VERSION),kairos-cli)

kairos-agent-fips_URL := $(call URL_TEMPLATE,kairos-agent,$(AGENT_VERSION),kairos-agent,-fips)
immucore-fips_URL := $(call URL_TEMPLATE,immucore,$(IMMUCORE_VERSION),immucore,-fips)
kcrypt-discovery-challenger-fips_URL := $(call URL_TEMPLATE,kcrypt-challenger,$(KCRYPT_CHALLENGER_VERSION),kcrypt-discovery-challenger,-fips)
kairos-cli-fips_URL := $(call URL_TEMPLATE,provider-kairos,$(AGENT_PROVIDER_VERSION),kairos-cli,-fips)

.PHONY: all prepare download compress cleanup

all: prepare download compress cleanup

# Clean the output directory
prepare:
	@echo "Cleaning up the output directory..."
	@rm -rf $(OUTPUT_DIR)
	@if [ -z "$(SKIP_UPX)" ] && ! command -v upx >/dev/null 2>&1; then \
		echo "Error: upx binary is not available. Please install upx."; \
		exit 1; \
	fi

# Ensure the bundled directory exists
$(OUTPUT_DIR):
	@echo "Creating directory $(OUTPUT_DIR)..."
	@mkdir -p $(OUTPUT_DIR)

# Download all binaries (standard and FIPS)
download: $(addprefix $(OUTPUT_DIR)/, $(BINARY_NAMES)) $(addprefix $(OUTPUT_DIR_FIPS)/, $(addsuffix -fips, $(BINARY_NAMES)))
	@# Download edgevpn by itself
	@echo "Downloading and extracting edgevpn for architecture $(ARCH)..."
	@mkdir -p $(OUTPUT_DIR)
	@# Unfortunately edgevpn uses x86_64 instead of amd64 so we need to do some string manipulation here
	@curl -L -s https://github.com/mudler/edgevpn/releases/download/$(EDGEVPN_VERSION)/edgevpn-$(EDGEVPN_VERSION)-Linux-$(shell uname -m | sed -e 's/aarch64/arm64/').tar.gz | tar -xz -C $(OUTPUT_DIR)

# Download each binary
$(OUTPUT_DIR)/%:
	@echo "Downloading and extracting $* for architecture $(ARCH)..."
	@mkdir -p $(OUTPUT_DIR)
	@curl -L -s $($*_URL) | tar -xz -C $(OUTPUT_DIR)

# Download each FIPS binary
$(OUTPUT_DIR_FIPS)/%-fips:
	@echo "Downloading and extracting $*-fips for architecture $(ARCH)..."
	@mkdir -p $(OUTPUT_DIR_FIPS)
	@curl -L -s $($*-fips_URL) | tar -xz -C $(OUTPUT_DIR_FIPS)


# Run upx to compress binaries unless SKIP_UPX is set
compress:
	@if [ -z "$(SKIP_UPX)" ]; then \
		echo "Running upx compress..."; \
		upx -q -5 $(addprefix $(OUTPUT_DIR)/, $(BINARY_NAMES) edgevpn ); \
		upx -q -5 $(addprefix $(OUTPUT_DIR_FIPS)/, $(BINARY_NAMES)); \
	else \
		echo "Skipping upx compression as SKIP_UPX is set"; \
	fi
# Remove non-binary files from the output directory
cleanup:
	@echo "Cleaning up non-binary files..."
	@find $(OUTPUT_DIR) -type f ! -exec file {} \; | grep -v "executable" | awk -F: '{print $$1}' | xargs -r rm -f
	@find $(OUTPUT_DIR_FIPS) -type f ! -exec file {} \; | grep -v "executable" | awk -F: '{print $$1}' | xargs -r rm -f