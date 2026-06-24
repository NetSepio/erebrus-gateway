# Erebrus v2 gateway control plane (cmd/gateway). No v1 Aptos bootstrap.
FROM golang:bookworm AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o gateway ./cmd/gateway

FROM debian:bookworm-slim
WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/gateway .
ARG version=2.0.0
ENV VERSION=$version
EXPOSE 8080 9001
ENTRYPOINT ["./gateway"]