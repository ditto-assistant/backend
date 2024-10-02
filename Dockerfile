# syntax=docker/dockerfile:1.10.0
FROM golang:1.23-bullseye AS builder

# Set working directory
WORKDIR /workspace

# Install build dependencies using apt-get instead of apk
RUN apt-get update && apt-get install -y build-essential

# Copy your Go project code
COPY . .

# Install Go dependencies
RUN go mod download

# Build your Go application
RUN CGO_ENABLED=1 GOOS=linux CGO_LDFLAGS="-ldl" go build -o cmd/backend .

# Switch to a minimal runtime image
FROM debian:bullseye-slim

# Copy the compiled binary
COPY --from=builder /workspace/cmd/backend /app/backend

# Set working directory
WORKDIR /app

# Expose port
EXPOSE 3400

# Set the default command to run your application
CMD ["./backend"]