FROM node:24.14.0-alpine AS web-build
WORKDIR /src
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./web/
RUN corepack enable && corepack prepare pnpm@11.9.0 --activate && pnpm --dir web install --frozen-lockfile
COPY web ./web
RUN pnpm --dir web build:app

FROM golang:1.26.2-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-build /src/internal/webassets/dist ./internal/webassets/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/flowlens ./cmd/flowlens

FROM scratch
COPY --from=build /out/flowlens /flowlens
USER 10001:10001
ENTRYPOINT ["/flowlens"]
CMD ["serve"]
