# Erebrus gateway v2 control plane (cmd/gateway).

run:
	go run ./cmd/gateway

build:
	CGO_ENABLED=0 go build -o gateway ./cmd/gateway

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy

.PHONY: run build test vet tidy
