# Build stage
FROM golang:alpine AS build-app
WORKDIR /app
RUN apk update && apk add --no-cache git
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY . .
RUN go build -o erebrus-gateway .

FROM alpine AS final
WORKDIR /app
COPY --from=build-app /app/erebrus-gateway .
CMD ["./erebrus-gateway"]