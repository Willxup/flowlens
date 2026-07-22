# syntax=docker/dockerfile:1.7

FROM golang:1.26.2-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY test/release/fixture ./test/release/fixture
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/fixture ./test/release/fixture

FROM scratch
COPY --from=build /out/fixture /fixture
COPY test/fixtures/clashapi /fixtures
USER 10001:10001
ENTRYPOINT ["/fixture"]
