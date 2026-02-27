BINARY   := asura
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS  := -s -w -X main.version=$(VERSION)
GOFLAGS  := -trimpath

TAILWIND := ./tailwindcss

.PHONY: all build css watch test lint run clean hash-key release

all: build

css:
	$(TAILWIND) -i web/tailwind.input.css -o web/static/tailwind.css --minify

watch:
	$(TAILWIND) -i web/tailwind.input.css -o web/static/tailwind.css --watch

build:
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/asura

test:
	go test -race -count=1 ./...

lint:
	go vet ./...

run: build
	./$(BINARY) -config config.yaml

clean:
	rm -f $(BINARY)
	rm -rf dist

hash-key:
	@read -p "Enter API key: " key; \
	go run ./cmd/asura -hash-key "$$key"

release:
	@mkdir -p dist
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o dist/asura-linux-amd64 ./cmd/asura
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o dist/asura-linux-arm64 ./cmd/asura
	@echo "Binaries written to dist/"
