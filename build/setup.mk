# Ensure that go is installed. Note that this is independent of whether or not a server is being
# built, since the build script itself uses go.
ifeq ($(GO),)
    $(error "go is not available: see https://golang.org/doc/install")
endif

# Gather build variables to inject into the manifest tool.
BUILD_HASH_SHORT = $(shell git rev-parse --short HEAD)
BUILD_TAG_LATEST = $(shell git describe --tags --match 'v*' --abbrev=0 2>/dev/null)
BUILD_TAG_CURRENT = $(shell git tag --points-at HEAD)

# Ensure that the manifest tool is compiled. Go's caching makes this quick.
$(shell cd build/manifest && $(GO) build -ldflags '-X "main.BuildHashShort=$(BUILD_HASH_SHORT)" -X "main.BuildTagLatest=$(BUILD_TAG_LATEST)" -X "main.BuildTagCurrent=$(BUILD_TAG_CURRENT)"' -o ../bin/manifest)

# Extract the plugin id from the manifest.
PLUGIN_ID ?= $(shell build/bin/manifest id)
ifeq ($(PLUGIN_ID),)
    $(error "Cannot parse plugin id from plugin.json")
endif

# Extract the plugin version from the manifest.
PLUGIN_VERSION ?= $(shell build/bin/manifest version)
ifeq ($(PLUGIN_VERSION),)
    $(error "Cannot parse plugin version from plugin.json")
endif

# Determine if a server is defined in the manifest.
HAS_SERVER ?= $(shell build/bin/manifest has_server)

# Determine if a webapp is defined in the manifest.
HAS_WEBAPP ?= $(shell build/bin/manifest has_webapp)

# Store the current path for later use.
PWD ?= $(shell pwd)
