BIN     := bin/strig
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/MikePors/strigidae/internal/version.Version=$(VERSION)
# Each tool lives in its own module under tools/ so their dependency graphs never conflict.
tool = go tool -modfile=tools/$(1)/go.mod $(1)

.PHONY: build test lint fmt fmt-check tidy tidy-check vuln actionlint check run clean

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/strig

test:
	go test -race ./...

lint:
	$(call tool,golangci-lint) run ./...

fmt:
	$(call tool,golangci-lint) fmt ./...

fmt-check:
	$(call tool,golangci-lint) fmt --diff ./...

tidy:
	go mod tidy
	for d in tools/*/; do (cd $$d && go mod tidy) || exit 1; done

tidy-check:
	go mod tidy -diff
	for d in tools/*/; do (cd $$d && go mod tidy -diff) || exit 1; done

vuln:
	$(call tool,govulncheck) ./...

actionlint:
	$(call tool,actionlint)

# Everything CI runs. Must pass before opening a PR.
check: fmt-check lint test tidy-check vuln actionlint build

run: build
	./$(BIN) $(ARGS)

clean:
	rm -rf bin
