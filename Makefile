# Erebrus gateway v2 control plane (cmd/gateway).

run:
	go run ./cmd/gateway

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build:
	CGO_ENABLED=0 go build -ldflags "-X github.com/NetSepio/gateway/internal/version.Version=$(VERSION)" -o gateway ./cmd/gateway

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy

.PHONY: run build test vet tidy
