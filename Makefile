BINARY     := automergent
MODULE     := github.com/iSundram/Automergent
VERSION    := $(shell cat VERSION 2>/dev/null || echo "0.0.0")
COMMIT     := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS_AUTOMERGENT := -s -w \
  -X '$(MODULE)/internal/version.Version=$(VERSION)' \
  -X '$(MODULE)/internal/version.Commit=$(COMMIT)' \
  -X '$(MODULE)/internal/version.BuildDate=$(BUILD_DATE)'

.PHONY: all build clean test lint fmt tidy install ci

all: build

build:
	CGO_ENABLED=1 go build -ldflags "$(LDFLAGS_AUTOMERGENT)" -o bin/$(BINARY) ./cmd/automergent

install:
	CGO_ENABLED=1 go install -ldflags "$(LDFLAGS_AUTOMERGENT)" ./cmd/automergent

clean:
	rm -rf bin/

test:
	CGO_ENABLED=1 go test ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .
	goimports -w .

tidy:
	go mod tidy

ci:
	CGO_ENABLED=1 go test ./...
	go vet ./...
	golangci-lint run ./...

.PHONY: release
release:
	AUTOMERGENT_VERSION=$(VERSION) goreleaser release --snapshot --clean --skip=validate -f dist/goreleaser-automergent.yaml
