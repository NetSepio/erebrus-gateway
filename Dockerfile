FROM golang:alpine AS build-app
WORKDIR /app
RUN apk update && apk add --no-cache git
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY . .
RUN go build -o erebrus-gateway . && apk del git
CMD ["./erebrus-gateway"]