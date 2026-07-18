BINARY     := wa
CMD_DIR    := .
BIN_DIR    := ./bin
PKG        := github.com/codebyoketch/wa-cli/internal/version

VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).BuildDate=$(BUILD_DATE)

.PHONY: all build test lint fmt vet clean run install

all: build

build:
	mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) $(CMD_DIR)

run: build
	$(BIN_DIR)/$(BINARY)

install:
	go install -ldflags "$(LDFLAGS)" $(CMD_DIR)

test:
	go test -race -cover ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

lint:
	golangci-lint run ./...

clean:
	rm -rf $(BIN_DIR)
