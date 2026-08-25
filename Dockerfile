# syntax=docker/dockerfile:1

# ---------- Build stage ----------
FROM golang:1.26.5-alpine AS builder

WORKDIR /src

# Cache module downloads
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code and build a static binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/ctxhive .

# ---------- Runtime stage ----------
FROM alpine:3.22

RUN adduser -D -u 10001 appuser

COPY --from=builder /out/ctxhive /usr/local/bin/ctxhive

USER appuser
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/ >/dev/null 2>&1 || exit 1

ENTRYPOINT ["ctxhive"]