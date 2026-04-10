BINDIR  := bin
ENVOKE  := $(BINDIR)/envoke
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

.PHONY: build build-envoke test test-race test-cover test-e2e lint fmt fmt-check shellcheck tidy govulncheck gosec clean install release

build: build-envoke

build-envoke:
	@mkdir -p $(BINDIR)
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(ENVOKE) ./cmd/envoke

test:
	go test ./...

test-race:
	CGO_ENABLED=1 go test -race ./...

test-cover:
	go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out -o coverage.html

test-e2e:
	go test -tags=e2e ./cmd/envoke/...

lint:
	go vet ./...

fmt:
	gofmt -w .

fmt-check:
	@unformatted=$$(go list -f '{{.Dir}}' ./... | xargs gofmt -l); if [ -n "$$unformatted" ]; then echo "The following files need formatting:"; echo "$$unformatted"; exit 1; fi

shellcheck:
	find snippets -name '*.sh' -print0 | xargs -0 shellcheck --severity=warning

tidy:
	go mod tidy && go mod verify

govulncheck:
	govulncheck ./...

# G304 — file inclusion via variable: intentional, reads user-supplied config/.env paths
# G104 — unhandled errors: project convention allows best-effort cleanup (slog.Warn)
# G204 — subprocess with variable: by design, this tool runs bw/vault with user args
# G706 — log injection via slog: slog is not susceptible to this injection vector
# G703 — path traversal: os.Remove calls are guarded by IsManaged()/isManagedKubeconfig()
# G115 — integer overflow: int(fd) safe (fds are small non-negative), uint32(pid) safe
gosec:
	gosec -exclude=G304,G104,G204,G706,G703,G115 -fmt text -stdout -verbose=text ./...

clean:
	rm -rf $(BINDIR) coverage.out coverage.html

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/envoke

release:
	goreleaser build --snapshot --clean
