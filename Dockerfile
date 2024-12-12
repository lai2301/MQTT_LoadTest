# Build stage
FROM golang:1.21-alpine AS builder

# Install git and build dependencies
RUN apk add --no-cache git

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o mqtt-load-tester

# Final stage
FROM alpine:latest

# Add ca certificates for SSL/TLS
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/mqtt-load-tester .

# Set entrypoint
ENTRYPOINT ["./mqtt-load-tester"]

# Default command (can be overridden)
CMD ["--help"] 