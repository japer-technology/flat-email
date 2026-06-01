BINARY := flat-email
PKG := ./cmd/flat-email
PREFIX ?= /usr/local
GO ?= go

.PHONY: all build install test lint vet fmt clean

all: build

build:
	$(GO) build -o $(BINARY) $(PKG)

install:
	$(GO) install $(PKG)

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w .

lint:
	gofmt -l .
	$(GO) vet ./...

clean:
	rm -f $(BINARY)
	rm -rf dist
