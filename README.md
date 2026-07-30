<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="./assets/flowlens-brand-dark.svg" />
    <source media="(prefers-color-scheme: light)" srcset="./assets/flowlens-brand-light.svg" />
    <img src="./assets/flowlens-brand-light.svg" alt="FlowLens — Edge Telemetry" width="380" />
  </picture>
</p>

<h1 align="center">FlowLens</h1>

<p align="center">看见每一字节的去向。</p>

<p align="center">
  <a href="https://github.com/Willxup/flowlens/releases/latest"><img src="https://img.shields.io/github/v/release/Willxup/flowlens?style=flat-square" alt="Latest release" /></a>
  <a href="https://github.com/Willxup/flowlens/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/Willxup/flowlens/ci.yml?branch=main&amp;style=flat-square&amp;label=CI" alt="CI status" /></a>
  <a href="https://github.com/Willxup/flowlens/pkgs/container/flowlens"><img src="https://img.shields.io/badge/Docker-GHCR-2496ED?style=flat-square&amp;logo=docker&amp;logoColor=white" alt="Docker image on GHCR" /></a>
  <a href="https://github.com/Willxup/flowlens/pkgs/container/flowlens"><img src="https://img.shields.io/badge/platform-linux%2Famd64%20%7C%20linux%2Farm64-FCC624?style=flat-square&amp;logo=linux&amp;logoColor=black" alt="Linux amd64 and arm64" /></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.26.2-00ADD8?style=flat-square&amp;logo=go&amp;logoColor=white" alt="Go 1.26.2" /></a>
  <a href="./LICENSE"><img src="https://img.shields.io/github/license/Willxup/flowlens?style=flat-square" alt="MIT License" /></a>
</p>

<p align="center">
  <a href="#核心能力">核心能力</a> ·
  <a href="#快速开始">快速开始</a> ·
  <a href="./config/config.example.yaml">配置</a> ·
  <a href="./docs/operations.md">运维</a> ·
  <a href="./api/openapi.yaml">API</a>
</p>

<p align="center">
  面向 sing-box Clash API 的自托管流量仪表盘。<br />
  精确保存全局流量，并用连接维度解释流量去了哪里。
</p>

## 预览

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="./assets/flowlens-dark.png" />
    <source media="(prefers-color-scheme: light)" srcset="./assets/flowlens-light.png" />
    <img src="./assets/flowlens-light.png" alt="FlowLens 单页流量仪表盘" />
  </picture>
</p>

## 核心能力

| 能力       | 内容                                                    |
| ---------- | ------------------------------------------------------- |
| 实时吞吐   | 当前速度、1/5 分钟均值、60 分钟峰值与活动连接           |
| 历史趋势   | 今天、昨天、7/30/90 天、今年及自定义范围                |
| 多维归因   | 目标、Endpoint、端口、协议、来源网段与域名              |
| 数据可信度 | 明确展示归因覆盖、Top K 截断、缺口与恢复流量            |
| 可靠存储   | SQLite 多级聚合、容量保护、自动备份与校验恢复           |
| 轻量部署   | 单个 Go 服务嵌入 React 前端，scratch 镜像、非 root 运行 |

## 快速开始

推荐使用 Docker Compose。你需要一个已启用 Clash API 的 sing-box，并让它与 FlowLens 加入同一个 Docker 网络。

```bash
cp config/config.example.yaml config/config.yaml
mkdir -p data
docker network create flowlens_private
```

编辑 `config/config.yaml`，至少确认：

- `clash_api.url` 与 `clash_api.secret`
- `auth.access_key`（默认启用登录，至少 16 个字符）
- `time.timezone`（首次写入数据后不可修改）

启动服务：

```bash
docker compose -f docker-compose.example.yml pull
docker compose -f docker-compose.example.yml up -d
```

打开 [http://127.0.0.1:8080](http://127.0.0.1:8080)。示例 Compose 默认仅监听宿主机 loopback。

> Linux 文件权限、备份恢复、升级和故障排查见 [运维指南](./docs/operations.md)。完整配置说明直接写在 [`config/config.example.yaml`](./config/config.example.yaml) 中。

## 你需要知道的三件事

- 全局累计流量来自 `/connections` 计数器，是精确值；目标排行属于近似归因。
- 实时秒级样本只保存在内存中；历史查询来自 SQLite 聚合。
- FlowLens 不修改 sing-box 配置、路由、防火墙或代理连接。

## 文档

| 文档                                     | 用途                                   |
| ---------------------------------------- | -------------------------------------- |
| [配置示例](./config/config.example.yaml) | 全部配置项、默认值与安全说明           |
| [运维指南](./docs/operations.md)         | 部署、健康检查、备份、恢复、升级与排障 |
| [OpenAPI](./api/openapi.yaml)            | HTTP API 契约                          |
| [SSE 事件](./docs/api-sse.md)            | 实时事件与断线恢复语义                 |
| [安全策略](./SECURITY.md)                | 漏洞报告与安全边界                     |

## 开发

需要 Go 1.26.2、Node.js 24.14.0 和 pnpm 11.9.0。

```bash
corepack enable
make deps
make check
make frontend-e2e
```

## License

[MIT](./LICENSE)
