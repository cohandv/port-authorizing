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
# Including AWS CLI for EKS authentication, gcloud for GKE, and kubectl
RUN apk update && \
    apk upgrade && \
    apk add --no-cache \
        ca-certificates \
        tzdata \
        curl \
        python3 \
        py3-pip \
        groff \
        && \
    pip3 install --no-cache-dir --break-system-packages awscli && \
    rm -rf /var/cache/apk/* /root/.cache

# Trust AWS RDS's CA bundle so TLS to RDS Postgres/Redis verifies out of the box.
# Pinned to a known-good sha256 so a corrupted/tampered download fails the build
# instead of silently landing in the trust store. AWS rotates this bundle rarely
# (see README.md "Updating the pinned RDS CA bundle") - bump the hash by hand when it does.
RUN curl -fsSL -o /usr/local/share/ca-certificates/rds-global-bundle.crt \
        https://truststore.pki.rds.amazonaws.com/global/global-bundle.pem && \
    echo "90a6f1a7978a1b1b096dca6c57511ca48d3b8cf052f8e704baf358f34ea2f21b  /usr/local/share/ca-certificates/rds-global-bundle.crt" | sha256sum -c - && \
    update-ca-certificates

# Create non-root user
RUN addgroup -S portauth && adduser -S portauth -G portauth

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/port-authorizing /usr/local/bin/port-authorizing


# Create directories for data, kubeconfig, and AWS credentials
RUN mkdir -p /app/data /app/logs /home/portauth/.kube /home/portauth/.aws && \
    chown -R portauth:portauth /app /home/portauth

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
LABEL org.opencontainers.image.url="https://github.com/cohandv/port-authorizing"
LABEL org.opencontainers.image.source="https://github.com/cohandv/port-authorizing"
LABEL org.opencontainers.image.vendor="David Cohan"
LABEL org.opencontainers.image.licenses="MIT"

