# Build stage
FROM golang:1.21 AS builder
WORKDIR /app

# Force Go to use IPv4 and bypass proxy (fixes IPv6 timeouts)
ENV GODEBUG=netdns=go
ENV GOPROXY=direct

# Copy module files first (for caching)
COPY go.mod go.sum ./
RUN go mod download -x   # -x prints each download step

# Copy the rest of the source
COPY . .

# Build the binary (adjust path if your main.go is inside backend/)
RUN go build -o plastic-backend ./backend

# Runtime stage
FROM debian:bookworm-slim
WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/plastic-backend .

EXPOSE 8080
CMD ["./plastic-backend"]
