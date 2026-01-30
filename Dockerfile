# Stage 1: Build frontend
FROM node:20-alpine AS frontend-builder
RUN npm install -g pnpm
WORKDIR /app/frontend
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/ ./
RUN pnpm run build

# Stage 2: Build Go binary
FROM golang:1.22-alpine AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o server .

# Stage 3: Final runtime image
FROM alpine:latest
RUN apk add --no-cache bash curl

# Install hey
RUN curl -L https://hey-release.s3.us-east-2.amazonaws.com/hey_linux_amd64 -o /usr/local/bin/hey && \
    chmod +x /usr/local/bin/hey

WORKDIR /app
COPY --from=go-builder /app/server .
COPY --from=frontend-builder /app/frontend/out ./frontend/out
COPY production_stress_test.sh ./
RUN chmod +x production_stress_test.sh

EXPOSE 8090 8091

CMD ["./server"]
