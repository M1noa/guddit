BINARY  := guddit
VERSION := $(shell git describe --tags --dirty --always 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

GOFLAGS := -trimpath
GO      := go

.PHONY: all build run test fmt vet tidy clean install release-snapshot release smoke

all: build

build:
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/guddit

run: build
	./$(BINARY) $(ARGS)

test:
	$(GO) test ./...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

tidy:
	$(GO) mod tidy

clean:
	rm -f $(BINARY) $(BINARY).exe
	rm -rf bin dist

install: build
	$(GO) install $(GOFLAGS) -ldflags "$(LDFLAGS)" ./cmd/guddit

release-snapshot:
	goreleaser release --snapshot --clean

release:
	goreleaser release --clean

smoke: build
	@echo "==> homepage (anon, json)"
	@./$(BINARY) home --limit 3 --format json --no-color | head -c 600; echo
	@echo "==> r/golang top (pretty)"
	@./$(BINARY) sub golang top --limit 5 --no-color | head -40
	@echo "==> search"
	@./$(BINARY) search "go cobra cli" --limit 3 --no-color | head -40
