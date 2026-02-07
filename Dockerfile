# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git gcc musl-dev

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build CLI
RUN CGO_ENABLED=1 GOOS=linux go build -o kube-upgrade-advisor ./cmd/cli

# Build server
RUN CGO_ENABLED=1 GOOS=linux go build -o kube-upgrade-server ./cmd/server

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates curl

WORKDIR /app

# Copy binaries
COPY --from=builder /app/kube-upgrade-advisor .
COPY --from=builder /app/kube-upgrade-server .

# Copy knowledge base
COPY knowledge-base ./knowledge-base

# Create directory for database
RUN mkdir -p /data

# Default to CLI mode
ENV DATABASE_URL=/data/kube-advisor.db
ENV API_KNOWLEDGE_PATH=./knowledge-base/apis.json
ENV CHART_KNOWLEDGE_PATH=./knowledge-base/chart-matrix.json

ENTRYPOINT ["./kube-upgrade-advisor"]
CMD ["--help"]