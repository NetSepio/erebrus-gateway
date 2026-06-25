# Erebrus v2 gateway control plane (cmd/gateway). No v1 Aptos bootstrap.
FROM golang:bookworm AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG version=dev
ARG tag=unknown
RUN CGO_ENABLED=0 go build \
    -ldflags "-X github.com/NetSepio/gateway/internal/version.Version=${version} -X github.com/NetSepio/gateway/internal/version.Tag=${tag}" \
    -o gateway ./cmd/gateway

FROM debian:bookworm-slim
WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates wget && rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/gateway .
EXPOSE 8080
ENTRYPOINT ["./gateway"]