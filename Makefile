BIN     := bin/strig
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/MikePors/strigidae/internal/version.Version=$(VERSION)
LINT    := go tool -modfile=tools/go.mod golangci-lint

.PHONY: build test lint fmt run clean

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/strig

test:
	go test -race ./...

lint:
	$(LINT) run ./...

fmt:
	$(LINT) fmt ./...

run: build
	./$(BIN) $(ARGS)

clean:
	rm -rf bin
