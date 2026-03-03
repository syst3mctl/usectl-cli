BINARY_NAME = usectl
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_DIR = dist

LDFLAGS = -s -w -X main.Version=$(VERSION)

.PHONY: build clean release install

## Build for current platform
build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) .

## Install to /usr/local/bin
install: build
	sudo cp $(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)

## Build release binaries for all platforms
release: clean
	@mkdir -p $(BUILD_DIR)
	GOOS=linux  GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .
	GOOS=linux  GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 .
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 .
	@echo "✓ Release binaries in $(BUILD_DIR)/"
	@ls -lh $(BUILD_DIR)/

clean:
	rm -rf $(BUILD_DIR) $(BINARY_NAME)
