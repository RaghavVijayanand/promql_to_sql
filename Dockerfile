# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git make

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN make build

# Runtime stage
FROM alpine:3.20

RUN apk --no-cache add ca-certificates \
    && adduser -D -u 1001 appuser

WORKDIR /home/appuser

# Copy binary from builder
COPY --from=builder /app/bin/promql-transpiler .

USER appuser

# Expose port if needed for future web interface
EXPOSE 8080

ENTRYPOINT ["./promql-transpiler"]
