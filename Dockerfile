FROM golang:1.23-alpine AS builder

# Install dependencies (including gcc)
RUN apk add --no-cache gcc libc-dev

# Set working directory
WORKDIR /workspace

# Copy your Go project code
COPY . .

# Install Go dependencies
RUN go mod download

# Build your Go application (main is in project root)
RUN go build -o cmd/backend .

# Switch to a minimal runtime image
FROM alpine:latest

# Copy the compiled binary
COPY --from=builder /workspace/cmd/backend /app/backend

# Set working directory
WORKDIR /app

# Expose port
EXPOSE 3400

# Set the default command to run your application
CMD ["./backend"]