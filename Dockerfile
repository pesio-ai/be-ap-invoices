# Build stage
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd/server

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

# Create non-root user
RUN addgroup -g 1000 pesio && \
    adduser -D -u 1000 -G pesio pesio

WORKDIR /home/pesio

# Copy binary from builder
COPY --from=builder /app/main .

# Copy migrations
COPY --from=builder /app/migrations ./migrations

# Change ownership
RUN chown -R pesio:pesio /home/pesio

# Switch to non-root user
USER pesio

# Expose port
EXPOSE 8085

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8085/health || exit 1

# Run the application
CMD ["./main"]
