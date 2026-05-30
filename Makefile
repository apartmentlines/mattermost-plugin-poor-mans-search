GO ?= go
GOIMPORTS ?= $(shell command -v goimports 2> /dev/null)
GO_TEST_FLAGS ?= -race
GO_BUILD_FLAGS ?=
MM_DEBUG ?=
PLUGIN_ID := com.mattermost.plugin-poor-mans-search
PLUGIN_VERSION ?= 0.0.0-dev
BUNDLE_NAME ?= $(PLUGIN_ID)-$(PLUGIN_VERSION).tar.gz
DLV_DEBUG_PORT := 2346
DEFAULT_GOOS := $(shell $(GO) env GOOS)
DEFAULT_GOARCH := $(shell $(GO) env GOARCH)
export GOCACHE := $(PWD)/.cache/go-build
export GOPATH := $(PWD)/.cache/gopath
export GOBIN ?= $(PWD)/build/bin
export GO111MODULE=on

ifneq ($(MM_DEBUG),)
	GO_BUILD_GCFLAGS = -gcflags "all=-N -l"
else
	GO_BUILD_GCFLAGS =
endif

## Define the default target (make all)
.PHONY: default
default: all

## Checks the code style, tests, builds and bundles the plugin.
.PHONY: all
all: check-style test dist

## Ensures the plugin manifest is valid
.PHONY: manifest-check
manifest-check:
	@test -f plugin.json
	@$(GO) test ./server -run '^$$' >/dev/null

## Propagates plugin manifest information into the server/ and webapp/ folders.
.PHONY: apply
apply:
	@echo "No generated manifest files to apply for this plugin."

## Install go tools
.PHONY: install-go-tools
install-go-tools:
	@echo Installing go tools
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.7.2
	$(GO) install gotest.tools/gotestsum@v1.13.0

## Runs eslint and golangci-lint
.PHONY: check-style
check-style: manifest-check
	$(GO) vet ./...
	@test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './.cache/*' -not -path './dist/*' -not -path './server/dist/*'))" || \
		(echo "Go files need formatting; run make format"; exit 1)
	@if [ -x "$(GOBIN)/golangci-lint" ]; then \
		$(GOBIN)/golangci-lint run ./...; \
	else \
		echo "Skipping golangci-lint; run make install-go-tools to install it."; \
	fi

## Builds the server, if it exists, for all supported architectures, unless MM_SERVICESETTINGS_ENABLEDEVELOPER is set.
.PHONY: server
server:
	mkdir -p server/dist
ifneq ($(MM_DEBUG),)
	$(info DEBUG mode is on; to disable, unset MM_DEBUG)
endif
ifneq ($(MM_SERVICESETTINGS_ENABLEDEVELOPER),)
	@echo Building plugin only for $(DEFAULT_GOOS)-$(DEFAULT_GOARCH) because MM_SERVICESETTINGS_ENABLEDEVELOPER is enabled
	GOOS=$(DEFAULT_GOOS) GOARCH=$(DEFAULT_GOARCH) $(GO) build $(GO_BUILD_FLAGS) $(GO_BUILD_GCFLAGS) -trimpath -o server/dist/plugin-$(DEFAULT_GOOS)-$(DEFAULT_GOARCH) ./server
else
	GOOS=linux GOARCH=amd64 $(GO) build $(GO_BUILD_FLAGS) $(GO_BUILD_GCFLAGS) -trimpath -o server/dist/plugin-linux-amd64 ./server
	GOOS=linux GOARCH=arm64 $(GO) build $(GO_BUILD_FLAGS) $(GO_BUILD_GCFLAGS) -trimpath -o server/dist/plugin-linux-arm64 ./server
	GOOS=darwin GOARCH=amd64 $(GO) build $(GO_BUILD_FLAGS) $(GO_BUILD_GCFLAGS) -trimpath -o server/dist/plugin-darwin-amd64 ./server
	GOOS=darwin GOARCH=arm64 $(GO) build $(GO_BUILD_FLAGS) $(GO_BUILD_GCFLAGS) -trimpath -o server/dist/plugin-darwin-arm64 ./server
	GOOS=windows GOARCH=amd64 $(GO) build $(GO_BUILD_FLAGS) $(GO_BUILD_GCFLAGS) -trimpath -o server/dist/plugin-windows-amd64.exe ./server
endif

## Builds the webapp, if it exists.
.PHONY: webapp
webapp:
	mkdir -p webapp/dist
	cp webapp/src/plugin.js webapp/dist/main.js

## Generates a tar bundle of the plugin for install.
.PHONY: bundle
bundle: server webapp
	rm -rf dist
	mkdir -p dist/$(PLUGIN_ID)/server
	mkdir -p dist/$(PLUGIN_ID)/webapp
	cp plugin.json dist/$(PLUGIN_ID)/
	cp -r server/dist dist/$(PLUGIN_ID)/server/
	cp -r webapp/dist dist/$(PLUGIN_ID)/webapp/
	cd dist && tar -czf $(BUNDLE_NAME) $(PLUGIN_ID)
	@echo plugin built at: dist/$(BUNDLE_NAME)

## Builds and bundles the plugin.
.PHONY: dist
dist: bundle

$(GOBIN)/pluginctl: $(wildcard build/pluginctl/*.go)
	mkdir -p $(GOBIN)
	$(GO) build -o $(GOBIN)/pluginctl ./build/pluginctl

## Builds and installs the plugin to a server.
.PHONY: deploy
deploy: $(GOBIN)/pluginctl dist
	$(GOBIN)/pluginctl deploy $(PLUGIN_ID) dist/$(BUNDLE_NAME)

## Builds and installs the plugin to a server, updating the webapp automatically when changed.
.PHONY: watch
watch:
	@echo "watch is not implemented for this static webapp yet; use make deploy."

## Installs a previous built plugin with updated webpack assets to a server.
.PHONY: deploy-from-watch
deploy-from-watch: $(GOBIN)/pluginctl bundle
	$(GOBIN)/pluginctl deploy $(PLUGIN_ID) dist/$(BUNDLE_NAME)

## Setup dlv for attaching, identifying the plugin PID for other targets.
.PHONY: setup-attach
setup-attach:
	$(eval PLUGIN_PID := $(shell ps aux | grep "plugins/${PLUGIN_ID}" | grep -v "grep" | awk -F " " '{print $$2}'))
	$(eval NUM_PID := $(shell echo -n ${PLUGIN_PID} | wc -w))
	@if [ ${NUM_PID} -gt 2 ]; then \
		echo "** There is more than 1 plugin process running. Run 'make kill reset' to restart just one."; \
		exit 1; \
	fi

## Check if setup-attach succeeded.
.PHONY: check-attach
check-attach:
	@if [ -z "${PLUGIN_PID}" ]; then \
		echo "Could not find plugin PID; the plugin is not running. Exiting."; \
		exit 1; \
	else \
		echo "Located plugin running with PID: ${PLUGIN_PID}"; \
	fi

## Attach dlv to an existing plugin instance.
.PHONY: attach
attach: setup-attach check-attach
	dlv attach ${PLUGIN_PID}

## Attach dlv to an existing plugin instance, exposing a headless instance on $DLV_DEBUG_PORT.
.PHONY: attach-headless
attach-headless: setup-attach check-attach
	dlv attach ${PLUGIN_PID} --listen :$(DLV_DEBUG_PORT) --headless=true --api-version=2 --accept-multiclient

## Detach dlv from an existing plugin instance, if previously attached.
.PHONY: detach
detach: setup-attach
	@DELVE_PID=$$(ps aux | grep "dlv attach ${PLUGIN_PID}" | grep -v "grep" | awk -F " " '{print $$2}'); \
	if [ -n "$$DELVE_PID" ]; then \
		echo "Located existing delve process running with PID: $$DELVE_PID. Killing."; \
		kill -9 $$DELVE_PID; \
	fi

## Runs any lints and unit tests defined for the server and webapp, if they exist.
.PHONY: test
test: apply
	mkdir -p $(GOCACHE) $(GOPATH)
	$(GO) test $(GO_TEST_FLAGS) ./...

## Runs tests verbosely, showing output for each test.
.PHONY: vtest
vtest: apply
	mkdir -p $(GOCACHE) $(GOPATH)
	$(GO) test $(GO_TEST_FLAGS) -v ./...

## Runs any lints and unit tests defined for the server and webapp, if they exist, optimized for a CI environment.
.PHONY: test-ci
test-ci: apply
	mkdir -p $(GOCACHE) $(GOPATH)
	$(GO) test $(GO_TEST_FLAGS) -v ./...

## Creates a coverage report for the server code.
.PHONY: coverage
coverage: apply
	mkdir -p server
	$(GO) test $(GO_TEST_FLAGS) -coverprofile=server/coverage.txt ./server/...
	$(GO) tool cover -html=server/coverage.txt -o server/coverage.html
	@echo coverage report written to server/coverage.html

## Extract strings for translation from the source code.
.PHONY: i18n-extract
i18n-extract:
	@echo "i18n extraction is not wired for this plugin yet."

## Disable the plugin.
.PHONY: disable
disable: $(GOBIN)/pluginctl detach
	$(GOBIN)/pluginctl disable $(PLUGIN_ID)

## Enable the plugin.
.PHONY: enable
enable: $(GOBIN)/pluginctl
	$(GOBIN)/pluginctl enable $(PLUGIN_ID)

## Reset the plugin, effectively disabling and re-enabling it on the server.
.PHONY: reset
reset: $(GOBIN)/pluginctl detach
	$(GOBIN)/pluginctl reset $(PLUGIN_ID)

## Kill all instances of the plugin, detaching any existing dlv instance.
.PHONY: kill
kill: detach
	$(eval PLUGIN_PID := $(shell ps aux | grep "plugins/${PLUGIN_ID}" | grep -v "grep" | awk -F " " '{print $$2}'))
	@for PID in ${PLUGIN_PID}; do \
		echo "Killing plugin pid $$PID"; \
		kill -9 $$PID; \
	done

## Clean removes all build artifacts.
.PHONY: clean
clean:
	rm -rf dist server/dist webapp/dist build/bin server/coverage.txt server/coverage.html

## Prints recent plugin log entries from Mattermost.
.PHONY: logs
logs: $(GOBIN)/pluginctl
	$(GOBIN)/pluginctl logs $(PLUGIN_ID)

## Watches plugin log entries from Mattermost.
.PHONY: logs-watch
logs-watch: $(GOBIN)/pluginctl
	$(GOBIN)/pluginctl logs-watch $(PLUGIN_ID)

.PHONY: help
help:
	@cat Makefile | grep -v '\.PHONY' | grep -v '\help:' | grep -B1 -E '^[a-zA-Z0-9_.-]+:.*' | sed -e "s/:.*//" | sed -e "s/^## //" | grep -v '\-\-' | sed '1!G;h;$$!d' | awk 'NR%2{printf "\033[36m%-30s\033[0m",$$0;next;}1' | sort

## Generates mocks with go generate.
.PHONY: mocks
mocks:
	$(GO) install github.com/golang/mock/mockgen@v1.6.0
	$(GO) generate ./...

## Formats Go code.
.PHONY: format
format:
	$(GO) fmt ./...
	@if [ -n "$(GOIMPORTS)" ]; then \
		$(GOIMPORTS) -local github.com/apartmentlines/mattermost-plugin-poor-mans-search -w .; \
	else \
		echo "Skipping goimports; binary not found."; \
	fi

.PHONY: patch minor major patch-rc minor-rc major-rc
## Bumps the patch version (semver).
patch:
	@echo "Release tagging is not wired for this repo yet."
	@exit 1

## Bumps the minor version (semver).
minor:
	@echo "Release tagging is not wired for this repo yet."
	@exit 1

## Bumps the major version (semver).
major:
	@echo "Release tagging is not wired for this repo yet."
	@exit 1

## Bumps the patch release candidate version (semver).
patch-rc:
	@echo "Release tagging is not wired for this repo yet."
	@exit 1

## Bumps the minor release candidate version (semver).
minor-rc:
	@echo "Release tagging is not wired for this repo yet."
	@exit 1

## Bumps the major release candidate version (semver).
major-rc:
	@echo "Release tagging is not wired for this repo yet."
	@exit 1
