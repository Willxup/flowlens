FROM golang:1.26.2-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/flowlens ./cmd/flowlens

FROM scratch
COPY --from=build /out/flowlens /flowlens
USER 10001:10001
ENTRYPOINT ["/flowlens"]
CMD ["serve"]
