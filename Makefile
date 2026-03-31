BINDIR := bin
RENV   := $(BINDIR)/renv
KCTX   := $(BINDIR)/kctx
GOFLAGS := -trimpath

.PHONY: build build-renv build-kctx test test-race test-cover lint fmt tidy clean install release

build: build-renv build-kctx

build-renv:
	@mkdir -p $(BINDIR)
	go build $(GOFLAGS) -o $(RENV) ./cmd/renv

build-kctx:
	@mkdir -p $(BINDIR)
	go build $(GOFLAGS) -o $(KCTX) ./cmd/kctx

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
	go install ./cmd/renv ./cmd/kctx

release:
	goreleaser build --snapshot --clean
