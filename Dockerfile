# ============================================================================
# Stage 1: Build Admin UI
# ============================================================================
FROM node:20-alpine AS admin-build

WORKDIR /build/admin

COPY admin/package.json admin/package-lock.json ./
RUN npm ci --ignore-scripts

COPY admin/ ./
RUN npm run build

# ============================================================================
# Stage 2: Build Go binary
# ============================================================================
FROM golang:1.26-alpine AS go-build

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Overwrite with the admin build output
COPY --from=admin-build /build/admin/dist ./admin/dist

RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w \
      -X github.com/wangling-miao/aroute/internal/version.Version=${VERSION} \
      -X github.com/wangling-miao/aroute/internal/version.Commit=${COMMIT} \
      -X github.com/wangling-miao/aroute/internal/version.BuildDate=${BUILD_DATE}" \
    -o /build/bin/aroute ./cmd/aroute

# ============================================================================
# Stage 3: Production runtime
# ============================================================================
FROM alpine:3.21 AS runtime

LABEL org.opencontainers.image.title="ARoute CMS" \
      org.opencontainers.image.description="Go-based microkernel CMS with plugin sandbox isolation" \
      org.opencontainers.image.source="https://github.com/wangling-miao/aroute" \
      org.opencontainers.image.vendor="ARoute" \
      org.opencontainers.image.licenses="Apache-2.0"

RUN addgroup -S aroute && adduser -S aroute -G aroute

COPY --from=go-build /build/bin/aroute /usr/local/bin/aroute

# Default config directory
RUN mkdir -p /data && chown aroute:aroute /data

VOLUME ["/data"]

USER aroute

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8080/healthz || exit 1

ENTRYPOINT ["aroute"]
CMD ["serve"]
