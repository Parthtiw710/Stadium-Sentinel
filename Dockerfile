# ──────────────────────────────────────────────
# Stage 1: Build React UI
# ──────────────────────────────────────────────
FROM node:24-alpine AS react-builder

WORKDIR /app/dashboard

# Install dependencies first (cached layer)
COPY dashboard/package*.json ./
RUN npm ci

# Copy source and build
COPY dashboard/ ./
RUN npm run build

# ──────────────────────────────────────────────
# Stage 2: Build Go Binary (with CGO for SQLite)
# ──────────────────────────────────────────────
FROM golang:1.26-bookworm AS go-builder

WORKDIR /app

# Install build dependencies for CGO (SQLite)
RUN apt-get update && apt-get install -y --no-install-recommends libsqlite3-dev && \
    rm -rf /var/lib/apt/lists/*

# Pre-download modules (cached layer)
COPY go.mod go.sum ./
RUN go mod download

# Copy entire Go project structure
COPY . .

# Copy the built React dist into the correct embed path
COPY --from=react-builder /app/dashboard/dist ./cmd/sentinel/dashboard/dist

# Build the single binary (CGO enabled for go-sqlite3)
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /app/sentinel \
    ./cmd/sentinel/

# ──────────────────────────────────────────────
# Stage 3: Minimal Runtime Image
# ──────────────────────────────────────────────
FROM debian:bookworm-slim AS runtime

# Install SQLite runtime libs (needed by go-sqlite3 CGO binary)
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates libsqlite3-0 && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy only the final binary
COPY --from=go-builder /app/sentinel .

# Port the HTTP server listens on
EXPOSE 8080

# Environment variables
ENV PORT=8080
# Set ADMIN_PHONE at runtime: docker run -e ADMIN_PHONE=+91XXXXXXXXXX
ENV ADMIN_PHONE=""

ENTRYPOINT ["./sentinel"]
