# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git make

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
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

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

# Default command (server mode)
ENTRYPOINT ["port-authorizing"]
CMD ["server", "--config", "/app/config.yaml"]

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/health || exit 1

# Labels
LABEL org.opencontainers.image.title="Port Authorizing"
LABEL org.opencontainers.image.description="Secure database access proxy with authentication and authorization"
LABEL org.opencontainers.image.source="https://github.com/yourusername/port-authorizing"
LABEL org.opencontainers.image.version="${VERSION}"

