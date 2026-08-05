<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="./assets/flowlens-brand-dark.svg" />
    <source media="(prefers-color-scheme: light)" srcset="./assets/flowlens-brand-light.svg" />
    <img src="./assets/flowlens-brand-light.svg" alt="FlowLens — Edge Telemetry" width="380" />
  </picture>
</p>

<p align="center">
  <a href="./README.md"><strong>English</strong></a> ｜ <a href="./README.zh.md">简体中文</a>
</p>

<h1 align="center">FlowLens</h1>

<p align="center">See where every byte goes.</p>

<p align="center">
  <a href="https://github.com/Willxup/flowlens/releases/latest"><img src="https://img.shields.io/github/v/release/Willxup/flowlens?style=flat-square" alt="Latest release" /></a>
  <a href="https://github.com/Willxup/flowlens/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/Willxup/flowlens/ci.yml?branch=main&amp;style=flat-square&amp;label=CI" alt="CI status" /></a>
  <a href="https://github.com/Willxup/flowlens/pkgs/container/flowlens"><img src="https://img.shields.io/badge/Docker-GHCR-2496ED?style=flat-square&amp;logo=docker&amp;logoColor=white" alt="Docker image on GHCR" /></a>
  <img src="https://img.shields.io/badge/Linux-amd64%20%7C%20arm64-FCC624?style=flat-square&amp;logo=linux&amp;logoColor=black" alt="Linux amd64 and arm64" />
  <a href="./LICENSE"><img src="https://img.shields.io/github/license/Willxup/flowlens?style=flat-square" alt="MIT License" /></a>
</p>

FlowLens is a self-hosted traffic dashboard for the sing-box Clash API. It keeps exact global traffic totals, explains traffic with sampled connection dimensions, and serves a responsive React interface from a single Go process backed by SQLite—without modifying sing-box, routing, firewall rules, or proxy connections.

> Release images use immutable version tags and the mutable `latest` tag. Production deployments should pin an immutable version tag or digest.

## Screenshots

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="./assets/flowlens-dark.png" />
    <source media="(prefers-color-scheme: light)" srcset="./assets/flowlens-light.png" />
    <img src="./assets/flowlens-light.png" alt="FlowLens traffic dashboard" />
  </picture>
</p>

## Features

- Track live upload and download throughput, moving averages, 60-minute peaks, and active connections
- Explore today, yesterday, 7/30/90-day, year-to-date, and custom historical ranges
- Attribute traffic by target, endpoint, port, protocol, source network, and hostname
- Separate exact global totals from approximate connection attribution, Top K truncation, and unattributed traffic
- Persist multi-resolution SQLite rollups with retention, capacity protection, integrity checks, and validated local backups
- Inspect runtime sessions, collection gaps, attribution coverage, and current storage health
- Protect the web interface with a shared-key session or explicitly enable trusted-LAN mode
- Run as a non-root, read-only, multi-architecture container with the React application embedded in one Go binary

## Quick Start

Docker Compose is the recommended production runtime. FlowLens also needs a sing-box instance with the Clash API enabled and a shared Docker network.

```bash
git clone https://github.com/Willxup/flowlens.git
cd flowlens

cp config/config.example.yaml config/config.yaml
mkdir -p data
docker network create flowlens_private
```

Edit `config/config.yaml` and configure:

- `clash_api.url` and `clash_api.secret` for the reachable sing-box Clash API
- `auth.access_key` with at least 16 characters while authentication is enabled
- `time.timezone` before the database receives its first traffic record

Attach sing-box to `flowlens_private`, then start FlowLens with an immutable release image:

```bash
export FLOWLENS_IMAGE=ghcr.io/willxup/flowlens:v0.2.5

docker compose -f docker-compose.example.yml pull
docker compose -f docker-compose.example.yml up -d
```

Open [http://127.0.0.1:8080](http://127.0.0.1:8080). The example Compose file listens on the host loopback interface only.

For production, provide HTTPS through a trusted reverse proxy, keep authentication enabled unless the network is explicitly trusted, and back up the complete `data` directory. Read the [operations guide](./docs/operations.md) before exposing or upgrading the service.

## Design Boundaries

| Area | Implementation |
| --- | --- |
| Runtime | One Go process with the embedded React application |
| Telemetry source | sing-box Clash API `/connections` snapshots over HTTP |
| Global traffic | Exact upload and download deltas from the Clash API counters |
| Attribution | Sampled connection dimensions with Top K, `_other`, and `_unattributed` buckets |
| History | Multi-resolution SQLite rollups in `/var/lib/flowlens` |
| Live view | In-memory one-second samples delivered to the browser over same-origin SSE |
| Source privacy | Full address, network prefix, or disabled; prefix mode by default |
| Platforms | `linux/amd64`, `linux/arm64` |

FlowLens is an observer. It does not configure sing-box, expose the Clash API, or change host networking. Do not run two FlowLens instances against the same writable data directory, and do not change the configured timezone after the database contains traffic data.

## Local Development

### Prerequisites

- Go 1.26.2
- Node.js 24.14.0
- pnpm 11.9.0

### Build and Verify

```bash
corepack enable
make deps
make check
make frontend-e2e
```

Run the deterministic frontend demo without a sing-box instance:

```bash
pnpm --dir web dev:demo
```

All project caches, tools, test reports, and temporary files remain under `.flowlens-dev/`.

## Documentation

| Topic | Guide |
| --- | --- |
| Complete configuration and security notes | [Configuration example](./config/config.example.yaml) |
| Docker deployment, health checks, backup, restore, upgrade, and troubleshooting | [Operations](./docs/operations.md) |
| Server-Sent Events and reconnect behavior | [SSE events](./docs/api-sse.md) |
| HTTP API contract | [OpenAPI](./api/openapi.yaml) |
| Vulnerability reporting and security boundaries | [Security policy](./SECURITY.md) |

## License

This project is open source under the [MIT License](./LICENSE).
