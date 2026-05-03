# Multi-stage Dockerfile for Go HTTP Load Balancer

# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o loadbalancer .

# Final stage
FROM alpine:3.18

# Install curl for health checks
RUN apk --no-cache add curl

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/loadbalancer .

# Copy default config
COPY config.yaml .

# Expose the port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=10s --timeout=5s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# Run the load balancer
ENTRYPOINT ["./loadbalancer"]
CMD ["-config", "config.yaml"]
