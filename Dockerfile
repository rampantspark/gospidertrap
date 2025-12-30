# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a \
    -o gospidertrap .

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 gospidertrap && \
    adduser -D -u 1000 -G gospidertrap gospidertrap

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/gospidertrap .

# Create data directory for persistence
RUN mkdir -p /app/data && \
    chown -R gospidertrap:gospidertrap /app

# Switch to non-root user
USER gospidertrap

# Expose default port
EXPOSE 8000

# Set default environment variables
ENV PORT=8000 \
    DATA_DIR=/app/data \
    RATE_LIMIT=10 \
    RATE_BURST=20

# Health check using nc (netcat) which is built into Alpine
# Checks if the server is listening on the configured port
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD nc -z localhost ${PORT} || exit 1

# Volume for persistent data
VOLUME ["/app/data"]

# Run the application
ENTRYPOINT ["/app/gospidertrap"]
CMD ["-p", "8000", "-d", "/app/data"]
