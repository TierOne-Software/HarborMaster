.PHONY: build clean test coverage install fmt vet lint

BINARY := hm
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/harbormaster

install:
	go install $(LDFLAGS) ./cmd/harbormaster

test:
	go test -race -v ./...

coverage:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	go tool cover -html=coverage.out -o coverage.html

clean:
	rm -f $(BINARY) coverage.out coverage.html
	go clean

fmt:
	go fmt ./...

vet:
	go vet ./...

# lint verifies only; it never modifies files. Use `make fmt` to reformat.
lint: vet
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt: the following files are not formatted:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed; skipping (see https://golangci-lint.run)"; \
	fi
