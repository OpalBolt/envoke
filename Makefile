BINDIR  := bin
RENV    := $(BINDIR)/renv
KCTX    := $(BINDIR)/kctx
GOFLAGS := -trimpath
export CGO_ENABLED=0

_BASE_VERSION := $(shell cat VERSION)
COMMIT        := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
VERSION       := $(shell \
	tag=$$(git describe --tags --exact-match 2>/dev/null); \
	dirty=$$(git status --porcelain 2>/dev/null); \
	if [ -n "$$tag" ] && [ -z "$$dirty" ]; then \
		echo "$${tag#v}"; \
	elif [ -n "$$dirty" ]; then \
		echo "$(_BASE_VERSION)-dev+$(COMMIT)-dirty"; \
	else \
		echo "$(_BASE_VERSION)-dev+$(COMMIT)"; \
	fi)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG     := github.com/eficode/secure-handling-of-secrets/internal/version
LDFLAGS := -s -w \
	-X $(PKG).Version=$(VERSION) \
	-X $(PKG).Commit=$(COMMIT) \
	-X $(PKG).BuildDate=$(DATE)

.PHONY: build build-renv build-kctx test test-race test-cover lint fmt tidy clean install release

build: build-renv build-kctx

build-renv:
	@mkdir -p $(BINDIR)
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(RENV) ./cmd/renv

build-kctx:
	@mkdir -p $(BINDIR)
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(KCTX) ./cmd/kctx

test:
	go test ./...

test-race:
	go test -race ./...

test-cover:
	go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out -o coverage.html

lint:
	go vet ./...

fmt:
	gofmt -w .

tidy:
	go mod tidy && go mod verify

clean:
	rm -rf $(BINDIR) coverage.out coverage.html

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/renv ./cmd/kctx

release:
	goreleaser build --snapshot --clean
