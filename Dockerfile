# syntax=docker/dockerfile:1.7

ARG TARGETOS=linux
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

FROM --platform=$BUILDPLATFORM node:24.14.0-alpine AS web-build
WORKDIR /src
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./web/
RUN corepack enable && corepack prepare pnpm@11.9.0 --activate && pnpm --dir web install --frozen-lockfile
COPY web ./web
RUN pnpm --dir web build:app

FROM --platform=$BUILDPLATFORM golang:1.26.2-alpine AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION
ARG COMMIT
ARG BUILD_DATE
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-build /src/internal/webassets/dist ./internal/webassets/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags="-s -w -X github.com/Willxup/flowlens/internal/buildinfo.Version=$VERSION -X github.com/Willxup/flowlens/internal/buildinfo.Commit=$COMMIT -X github.com/Willxup/flowlens/internal/buildinfo.BuildDate=$BUILD_DATE" \
    -o /out/flowlens ./cmd/flowlens

FROM scratch
ARG VERSION
ARG COMMIT
ARG BUILD_DATE
LABEL org.opencontainers.image.title="FlowLens" \
      org.opencontainers.image.description="Self-hosted sing-box traffic dashboard" \
      org.opencontainers.image.source="https://github.com/Willxup/flowlens" \
      org.opencontainers.image.version="$VERSION" \
      org.opencontainers.image.revision="$COMMIT" \
      org.opencontainers.image.created="$BUILD_DATE" \
      org.opencontainers.image.licenses="MIT"
COPY --from=build /out/flowlens /flowlens
USER 10001:10001
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/flowlens", "healthcheck"]
ENTRYPOINT ["/flowlens"]
CMD ["serve"]
