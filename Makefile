# v1 legacy entrypoint (main branch)
run-v1:
	go run cmd/main.go cmd/server.go

build-v1:
	GOOS=linux GOARCH=amd64 go build -o gateway cmd/main.go cmd/server.go

# v2 control plane (v2 branch) — run before any erebrus node is up
run:
	go run ./cmd/gateway

build:
	CGO_ENABLED=0 go build -o gateway ./cmd/gateway

test:
	go test ./...

.PHONY: run run-v1 build build-v1 test