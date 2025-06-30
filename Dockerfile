# Build stage
FROM golang:alpine AS builder

# Install build dependencies
RUN apk add --no-cache gcc musl-dev sqlite-dev

# Set working directory
WORKDIR /app

# Copy source code
COPY *.go ./

RUN go mod init github.com/amaumene/gostremiofr

RUN go mod tidy

# Build the application with static linking
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o gostremiofr .

# Runtime stage
FROM scratch

# Copy the binary from builder
COPY --from=builder /app/gostremiofr /gostremiofr

# Copy SSL certificates for HTTPS requests
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Create data directory
COPY --from=builder /tmp /tmp

# Expose port
EXPOSE 5000

# Set environment variables
ENV PORT=5000
ENV LOG_LEVEL=info
# DATABASE_PATH can be set to customize database location (default: ./streams.db)
# ENV DATABASE_PATH=/data/streams.db

# Run the application
ENTRYPOINT ["/gostremiofr"]
