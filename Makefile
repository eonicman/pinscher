.PHONY: build install test clean lint

VERSION ?= $(shell grep 'const version' cmd/pinscher/main.go | awk -F'"' '{print $$2}')
BINARY = pinscher
GO = go
GOFLAGS = -ldflags="-s -w -X main.version=$(VERSION)"

build:
	$(GO) build $(GOFLAGS) -o $(BINARY) ./cmd/pinscher/

install: build
	cp $(BINARY) /usr/local/bin/

test:
	$(GO) test -v ./...

lint:
	$(GO) vet ./...

clean:
	rm -f $(BINARY)

# Run on local machine to verify probe output
check: build
	./$(BINARY) report
	@echo "---"
	./$(BINARY) lint-boot

# Run the full test suite + vet + build
ci: lint test build
	@echo "All checks passed for v$(VERSION)"