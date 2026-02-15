# Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /build

# Copy shared libraries first
COPY be-lib-common/ ./be-lib-common/
COPY be-lib-proto/ ./be-lib-proto/

# Copy service files
COPY be-ap-invoices/ ./be-ap-invoices/

# Build from service directory
WORKDIR /build/be-ap-invoices

# Download dependencies
RUN go mod download

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd/server

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates curl

# Create non-root user
RUN addgroup -g 1000 pesio && \
    adduser -D -u 1000 -G pesio pesio

WORKDIR /home/pesio

# Copy binary from builder
COPY --from=builder /build/be-ap-invoices/main .

# Copy migrations
COPY --from=builder /build/be-ap-invoices/migrations ./migrations

# Change ownership
RUN chown -R pesio:pesio /home/pesio

# Switch to non-root user
USER pesio

# Expose ports
EXPOSE 8086 9085

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8086/health || exit 1

# Run the application
CMD ["./main"]
