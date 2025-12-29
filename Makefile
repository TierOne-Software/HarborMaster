.PHONY: build clean test install

BINARY := hm
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/harbormaster

install:
	go install $(LDFLAGS) ./cmd/harbormaster

test:
	go test -v ./...

clean:
	rm -f $(BINARY)
	go clean

fmt:
	go fmt ./...

vet:
	go vet ./...

lint: fmt vet
