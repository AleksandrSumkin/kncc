# ===================================================
# Stage 1: Build Frontend (React + TypeScript + Vite)
# ===================================================
FROM node:22-alpine AS frontend-builder
WORKDIR /app/web

# Install dependencies with caching
COPY web/package*.json ./
RUN npm install

# Build static assets
COPY web/ ./
RUN npm run build

# ===================================================
# Stage 2: Build Backend (Go CGO-free binary)
# ===================================================
FROM golang:1.27-alpine AS backend-builder
WORKDIR /app

RUN apk add --no-cache git ca-certificates tzdata

# Cache Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and frontend build artifacts
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY web/web.go ./web/web.go
COPY --from=frontend-builder /app/web/dist ./web/dist

# Compile static Linux binary without CGO
ARG VERSION=""
ARG COMMIT=""
ARG DATE=""

RUN set -e; \
    LDFLAGS="-s -w"; \
    if [ -n "$VERSION" ] && [ "$VERSION" != "dev" ]; then LDFLAGS="$LDFLAGS -X main.version=${VERSION}"; fi; \
    if [ -n "$COMMIT" ] && [ "$COMMIT" != "none" ]; then LDFLAGS="$LDFLAGS -X main.commit=${COMMIT}"; fi; \
    if [ -n "$DATE" ] && [ "$DATE" != "unknown" ]; then LDFLAGS="$LDFLAGS -X main.date=${DATE}"; fi; \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
      -trimpath \
      -ldflags="${LDFLAGS}" \
      -o /app/bin/kncc ./cmd/kncc

# ===================================================
# Stage 3: Minimal and Secure Runtime Image
# ===================================================
FROM alpine:3.21 AS runner

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -g 10001 -S kncc \
    && adduser -u 10001 -S kncc -G kncc -h /data -s /sbin/nologin \
    && mkdir -p /data \
    && chown -R kncc:kncc /data

WORKDIR /app
COPY --from=backend-builder /app/bin/kncc /app/kncc

USER 10001:10001

EXPOSE 8080

VOLUME ["/data"]

ENTRYPOINT ["/app/kncc"]
CMD ["-config", "/etc/kncc/config.yaml"]
