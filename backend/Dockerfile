# syntax=docker/dockerfile:1
FROM golang:1.23-alpine AS base
WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY . .

FROM base AS test
RUN go test ./...

FROM base AS builder
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/taskflow ./cmd/server

FROM alpine:3.21 AS runtime
RUN apk add --no-cache ca-certificates && addgroup -S taskflow && adduser -S -G taskflow taskflow
WORKDIR /app
COPY --from=builder /out/taskflow /taskflow
COPY migrations ./migrations
COPY swagger ./swagger
USER taskflow
EXPOSE 8080 50051
HEALTHCHECK --interval=10s --timeout=3s --start-period=20s --retries=5 CMD ["/taskflow", "healthcheck"]
ENTRYPOINT ["/taskflow"]
