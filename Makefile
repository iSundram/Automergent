BINARY     := automergent
MODULE     := github.com/iSundram/Automergent
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.1.0")
COMMIT     := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
  -X '$(MODULE)/internal/version.Version=$(VERSION)' \
  -X '$(MODULE)/internal/version.Commit=$(COMMIT)' \
  -X '$(MODULE)/internal/version.BuildDate=$(BUILD_DATE)'

.PHONY: all build clean test lint fmt tidy install

all: build

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/automergent

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/automergent

clean:
	rm -rf bin/

test:
	go test ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .
	goimports -w .

tidy:
	go mod tidy

.PHONY: release
release:
	GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-linux-amd64   ./cmd/automergent
	GOOS=darwin  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-darwin-amd64  ./cmd/automergent
	GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-darwin-arm64  ./cmd/automergent
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-windows-amd64.exe ./cmd/automergent
