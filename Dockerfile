# ── Stage 1: build ──────────────────────────────────────────────────────────
# Use the official Go image to compile a fully-static binary.
# CGO_ENABLED=0 + GOFLAGS=-trimpath produce a portable, reproducible artifact.
FROM golang:1.26-alpine AS builder

WORKDIR /src

# Copy dependency manifests first so Docker can cache the download layer.
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source and build.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags="-s -w" \
    -trimpath \
    -o /bin/api \
    ./cmd/api

# ── Stage 2: final image ─────────────────────────────────────────────────────
# distroless/static has no shell, no package manager, and a tiny attack surface.
# The only thing in the image is the binary and the CA bundle (needed for HTTPS
# outbound calls if the app ever makes them).
FROM gcr.io/distroless/static:nonroot

# Copy the compiled binary from the builder stage.
COPY --from=builder /bin/api /api

# The app listens on 8080 by default (HTTP_ADDR env var).
EXPOSE 8080

# Run as the non-root user baked into the distroless image.
USER nonroot:nonroot

ENTRYPOINT ["/api"]
