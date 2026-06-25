# Erebrus gateway v2 control plane (cmd/gateway).

run:
	go run ./cmd/gateway

VERSION ?= 2.0.$(shell git rev-list --count HEAD 2>/dev/null || echo 0)
TAG ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)

build:
	CGO_ENABLED=0 go build -ldflags "-X github.com/NetSepio/gateway/internal/version.Version=$(VERSION) -X github.com/NetSepio/gateway/internal/version.Tag=$(TAG)" -o gateway ./cmd/gateway

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy

.PHONY: run build test vet tidy
