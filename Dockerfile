# ---------- Backend (Go API) ----------
FROM golang:1.23-alpine AS backend-builder

# Install build dependencies for CGO compilation (sqlite)
RUN apk add --no-cache git build-base

WORKDIR /src

COPY backend/go.mod backend/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY backend/ ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 GOOS=linux go build -o /out/filabridge .

# ---------- Frontend (Next.js) ----------
FROM node:22-alpine AS web-builder

WORKDIR /src

COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci

COPY web/ ./
ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run build

# ---------- Runtime ----------
FROM node:22-alpine

RUN apk update && apk --no-cache --no-scripts add ca-certificates sqlite

WORKDIR /app

# Go API binary
COPY --from=backend-builder /out/filabridge ./filabridge

# Next.js standalone server (UI + proxy)
COPY --from=web-builder /src/.next/standalone ./web
COPY --from=web-builder /src/.next/static ./web/.next/static
COPY --from=web-builder /src/public ./web/public

COPY docker/entrypoint.sh ./entrypoint.sh
RUN chmod +x ./entrypoint.sh ./filabridge && mkdir -p /app/data

# Single external port: Next.js serves the UI on 5000 and proxies
# /api/* and /ws/* to the Go API on 127.0.0.1:5001.
EXPOSE 5000

ENV GIN_MODE=release
ENV FILABRIDGE_DB_PATH=/app/data
ENV BACKEND_URL=http://127.0.0.1:5001
ENV NODE_ENV=production

CMD ["./entrypoint.sh"]
