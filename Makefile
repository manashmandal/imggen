BINARY := imggen
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)"

.PHONY: all build test coverage clean install lint fmt help

all: build

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/imggen

test:
	go test -v -race ./...

coverage:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

clean:
	rm -f $(BINARY) coverage.out coverage.html

install:
	go install $(LDFLAGS) ./cmd/imggen

lint:
	@which golangci-lint > /dev/null || (echo "Install golangci-lint: https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run

fmt:
	go fmt ./...
	goimports -w .

help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  build     Build the binary (default)"
	@echo "  test      Run tests with race detection"
	@echo "  coverage  Generate coverage report"
	@echo "  clean     Remove build artifacts"
	@echo "  install   Install to GOPATH/bin"
	@echo "  lint      Run golangci-lint"
	@echo "  fmt       Format code"
	@echo "  help      Show this help"
