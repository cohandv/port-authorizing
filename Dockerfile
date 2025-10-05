# Build stage
FROM golang:1.24-alpine3.21 AS builder

WORKDIR /app

# Install build dependencies and update all packages to latest versions
RUN apk update && \
    apk upgrade && \
    apk add --no-cache git make

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build arguments for version info
ARG VERSION=dev
ARG BUILD_TIME=unknown
ARG GIT_COMMIT=unknown

# Build the unified binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X main.GitCommit=${GIT_COMMIT}" \
    -o /app/port-authorizing \
    ./cmd/port-authorizing

# Runtime stage
FROM alpine:3.21

# Update all packages and install runtime dependencies with latest security patches
RUN apk update && \
    apk upgrade && \
    apk add --no-cache ca-certificates tzdata curl && \
    rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -S portauth && adduser -S portauth -G portauth

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/port-authorizing /usr/local/bin/port-authorizing

# Copy example config
COPY config.example.yaml /app/config.example.yaml

# Create directories for data
RUN mkdir -p /app/data /app/logs && \
    chown -R portauth:portauth /app

# Switch to non-root user
USER portauth

# Expose API port
EXPOSE 8080

# Default command runs server, but can be overridden for client commands
# Server mode (default):  docker run -v ./config.yaml:/app/config.yaml port-authorizing
# Client mode examples:
#   docker run port-authorizing login -u admin -p password
#   docker run port-authorizing list
#   docker run port-authorizing connect mydb -l 5432
ENTRYPOINT ["port-authorizing"]
CMD ["server", "--config", "/app/config.yaml"]

# Health check (only relevant for server mode)
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/health || exit 1

# Labels
LABEL org.opencontainers.image.title="Port Authorizing"
LABEL org.opencontainers.image.description="Secure proxy for any service with authentication, authorization, and audit logging"
LABEL org.opencontainers.image.url="https://github.com/davidcohan/port-authorizing"
LABEL org.opencontainers.image.source="https://github.com/davidcohan/port-authorizing"
LABEL org.opencontainers.image.vendor="David Cohan"
LABEL org.opencontainers.image.licenses="MIT"

