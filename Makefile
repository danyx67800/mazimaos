SHELL := /bin/sh
VERSION := $(shell cat VERSION)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X github.com/danyx67800/mazimaos/daemon/internal/version.Version=$(VERSION) \
	-X github.com/danyx67800/mazimaos/daemon/internal/version.Commit=$(COMMIT) \
	-X github.com/danyx67800/mazimaos/daemon/internal/version.BuildDate=$(BUILD_DATE)

UPDATE_PUBLIC_KEY ?=
ifneq ($(strip $(UPDATE_PUBLIC_KEY)),)
LDFLAGS += -X github.com/danyx67800/mazimaos/daemon/internal/updater.EmbeddedPublicKey=$(UPDATE_PUBLIC_KEY)
endif

.PHONY: all build test test-go test-ui lint run dev clean e2e audit release

all: test build

build: bin/mazimad bin/mazimactl

bin/mazimad:
	mkdir -p bin
	cd daemon && go build -trimpath -ldflags '$(LDFLAGS)' -o ../bin/mazimad ./cmd/mazimad

bin/mazimactl:
	mkdir -p bin
	cd daemon && go build -trimpath -ldflags '$(LDFLAGS)' -o ../bin/mazimactl ./cmd/mazimactl

bin/mazimaota:
	mkdir -p bin
	cd daemon && go build -trimpath -ldflags '$(LDFLAGS)' -o ../bin/mazimaota ./cmd/mazimaota

test: test-go test-ui

test-go:
	cd daemon && go test ./...

test-ui:
	npm test

lint:
	cd daemon && go vet ./...
	cd daemon && gofmt -l . | tee /tmp/mazima-gofmt.txt; test ! -s /tmp/mazima-gofmt.txt

run: build
	./bin/mazimad --dev --listen 127.0.0.1:8088 --data-dir ./dev-data --ui-dir ./ui

dev:
	./scripts/dev.sh

e2e:
	npm run test:e2e

audit:
	./scripts/feature-audit.sh

release:
	./scripts/release.sh

clean:
	rm -rf bin data dev-data .dev-data .e2e-data dist test-results playwright-report coverage
