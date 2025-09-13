# Build stage
FROM golang:1.23.4-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ignite .

# Production stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates curl && \
    addgroup -g 1001 ignite && \
    adduser -D -s /bin/sh -u 1001 -G ignite ignite

# Set environment variables
ENV DB_PATH=/app/data/
ENV DB_FILE=ignite.db
ENV DB_BUCKET=dhcp
ENV BIOS_FILE=boot-bios/pxelinux.0
ENV EFI_FILE=boot-efi/syslinux.efi
ENV TFTP_DIR=/app/public/tftp
ENV HTTP_DIR=/app/public/http
ENV HTTP_PORT=8080
ENV PROV_DIR=/app/public/provision

WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /build/ignite .
COPY --chown=ignite:ignite ./public ./public

# Create data directory for database
RUN mkdir -p /app/data && chown ignite:ignite /app/data

# Switch to non-root user
USER ignite

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/status || exit 1

CMD ["./ignite"]