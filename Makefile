SHELL := /bin/bash

APP_NAME := floppy
MAIN := ./cmd/floppy
DIST := dist

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

PLATFORMS := \
	darwin/amd64 \
	darwin/arm64 \
	linux/amd64 \
	linux/arm64

.PHONY: build
build:
	go build -ldflags "$(LDFLAGS)" -o $(APP_NAME) $(MAIN)

.PHONY: clean
clean:
	rm -rf $(DIST) $(APP_NAME)

.PHONY: release
release: clean
	@mkdir -p $(DIST)
	@for platform in $(PLATFORMS); do \
		IFS=/ read -r GOOS GOARCH <<< "$$platform"; \
		OUT_DIR=$(DIST)/$(APP_NAME)-$${GOOS}-$${GOARCH}; \
		mkdir -p $$OUT_DIR; \
		echo "Building $$GOOS/$$GOARCH"; \
		GOOS=$$GOOS GOARCH=$$GOARCH go build -ldflags "$(LDFLAGS)" -o $$OUT_DIR/$(APP_NAME) $(MAIN); \
		tar -czf $(DIST)/$(APP_NAME)-$${GOOS}-$${GOARCH}.tar.gz -C $$OUT_DIR $(APP_NAME); \
		rm -rf $$OUT_DIR; \
	done
	@cd $(DIST) && shasum -a 256 *.tar.gz > $(APP_NAME)-checksums.txt
