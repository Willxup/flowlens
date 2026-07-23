# FlowLens SSE 事件

`GET /api/v1/live` 是 FlowLens 唯一的 Server-Sent Events 接口，用于把内存中的一秒速度样本和采集状态推送给同源前端。完整 HTTP 字段见 [`api/openapi.yaml`](../api/openapi.yaml)。

## 连接与认证

- `auth.enabled=true` 时，请求必须携带登录后得到的 `flowlens_session` Cookie。浏览器同源 `EventSource` 会自动携带该 Cookie。
- `auth.enabled=false` 时，可匿名建立连接，不要求 Cookie，也不受会话过期影响。
- 响应类型为 `text/event-stream`，禁止缓存和反向代理缓冲。
- 仅在 `auth.enabled=true` 时，会话缺失或过期返回 `401`；流建立后会每秒重新检查会话，过期即关闭连接。
- FlowLens 不内置 TLS。需要远程访问时，应只通过可信的 HTTPS 反向代理暴露 FlowLens。

## 事件

每条事件都有十进制 `id`、事件名和一行 JSON `data`。`sequence` 与 `id` 相同，并在单次连接中严格递增。

### `snapshot`

连接建立后的第一条事件。`samples` 是当前内存实时环中的有序样本，最多 3600 条；没有历史样本时是空数组。

```text
id: 1
event: snapshot
data: {"sequence":1,"samples":[{"timestamp":1784689200,"upload_bytes_per_second":1024,"download_bytes_per_second":4096,"active_connections":3,"status":"ok"}]}
```

### `status`

快照后立即发送一次，随后仅在采集状态改变时发送。`status` 为 `ok`、`degraded` 或 `failed`，`ready` 表示就绪检查是否通过。

```text
event: status
data: {"sequence":2,"status":"ok","reason":"ready","ready":true}
```

### `sample`

有时间戳更新的一秒样本时发送。速度和活动连接为非负整数；`status=degraded` 表示该样本跨越采集缺口或来源不可完整观测。

```text
event: sample
data: {"sequence":3,"sample":{"timestamp":1784689201,"upload_bytes_per_second":2048,"download_bytes_per_second":8192,"active_connections":4,"status":"ok"}}
```

### `heartbeat`

没有其他要求时也会约每 15 秒发送，用于保持连接并检测断线。

```text
event: heartbeat
data: {"sequence":4,"at":1784689215}
```

## 重连、事件 ID 与缺口

事件 ID 只在当前连接内有效。第一版不按 `Last-Event-ID` 做服务器端回放；浏览器重连后会得到新的 `snapshot`，序号重新从 1 开始。断线期间的实时样本只有仍位于 3600 条内存环中时才会出现在新快照中。

采集缺口不会被补成零流量。实时样本会标记为 `degraded`，持久化历史会记录数据质量事件；精确全局累计流量仍以 Clash API `/connections` 累计计数器为准。
