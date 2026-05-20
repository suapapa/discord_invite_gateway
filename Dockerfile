# Build stage
FROM golang:1.26.3-alpine AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source files including templates
COPY main.go main_test.go ./
COPY templates/ ./templates/

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o discord_invite_gateway main.go

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/discord_invite_gateway .

# Copy config file (must be provided at runtime)
COPY config_sample.yaml ./config.yaml

# Expose default port
EXPOSE 8080

# Run the application
CMD ["./discord_invite_gateway"]
