# FlowLens

FlowLens 是一个面向 sing-box Clash API 的本地流量观测服务。它使用 Go 和 SQLite 记录全局累计流量、实时速度与运行会话，在重启、采集间隔和计数器回退场景下保持清晰的数据语义。

> 当前仓库完成了 Stage 1 全局最小闭环，适合测试和继续开发。Web 仪表盘、历史查询、多维归因、保留聚合与自动备份尚未提供。

## 当前能力

- 通过 `/connections` 读取累计上传、下载流量。
- 通过 `/traffic` 读取实时上传、下载速度。
- 将全局流量按 10 秒桶原子写入 SQLite。
- 记录运行会话，识别 sing-box 计数器回退和采集缺口。
- 使用共享登录密钥和内存 Cookie 会话保护业务 API。
- 提供 `/healthz`、`/readyz` 和受保护的 `/api/v1/status`。
- 使用严格 YAML 配置，拒绝未知字段、无效值和额外 YAML 文档。
- 提供非 root、只读根文件系统和最小权限的 Docker Compose 边界。

## 运行要求

- Docker Engine 与 Docker Compose
- 已启用 Clash API 的 sing-box
- 一个由 FlowLens 与 sing-box 共同加入的 Docker 用户自定义网络

FlowLens 只使用 HTTP 访问同一私有网络中的 Clash API。不要把 Clash API 端口直接暴露到公网。

## 快速开始

1. 在 sing-box 中启用 Clash API，并设置 `external_controller` 与 `secret`。确保 FlowLens 可以通过 Docker 私有网络访问该地址。

2. 准备配置和数据目录：

   ```bash
   cp config/config.example.yaml config/config.yaml
   mkdir -p data
   ```

3. 编辑 `config/config.yaml`：

   - 将 `clash_api.url` 改为私有网络中可访问的 sing-box 服务地址。
   - 将 `clash_api.secret` 改为 sing-box Clash API 的真实 Secret。
   - 使用下面的命令生成至少 16 字符的 FlowLens 登录密钥，并填写到 `auth.access_key`：

     ```bash
     openssl rand -base64 16
     ```

   - 根据需要设置 `time.timezone`。

4. 在 Linux 上为固定容器用户设置配置和数据目录权限：

   ```bash
   sudo chown 10001:10001 config/config.yaml data
   chmod 600 config/config.yaml
   chmod 700 data
   ```

   FlowLens 容器以 `10001:10001` 运行，必须能够读取配置并写入数据目录。使用 Docker Desktop 时，请按其文件共享权限模型确认这两个挂载可读写。

5. 创建 Compose 使用的私有网络，并让 sing-box 也加入该网络：

   ```bash
   docker network create flowlens_private
   ```

   如果该网络已经存在，请直接复用。`clash_api.url` 中的主机名应与 sing-box 在该网络中的服务名一致。

6. 构建并启动 FlowLens：

   ```bash
   docker compose -f docker-compose.example.yml up -d --build
   ```

7. 检查服务状态：

   ```bash
   curl -i http://127.0.0.1:8080/healthz
   curl -i http://127.0.0.1:8080/readyz
   ```

两个端点就绪时均返回 `204 No Content`。业务 API 需要先通过 `POST /api/v1/session` 使用 `auth.access_key` 登录并保留 `flowlens_session` Cookie。

## 配置与数据

- FlowLens 只读取 `/etc/flowlens/config.yaml`，不接受环境变量或命令行配置路径。
- 配置格式和字段说明见 [`config/config.example.yaml`](config/config.example.yaml)。当前 Stage 1 尚未执行 `storage.soft_limit`、`retention`、`privacy` 和 `backup` 策略，请保留合理配置并以“当前能力”列表为准。
- Compose 将真实配置只读挂载到容器，并把 SQLite 数据保存在 `./data/`。
- `config/config.yaml`、`data/`、开发缓存和内部计划均被 Git 忽略。

请勿提交真实 Clash API Secret、FlowLens 登录密钥、Cookie、数据库、备份或未脱敏日志。

## 数据语义

- `/connections` 是累计字节的唯一来源。
- `/traffic` 只用于速度采样，不能与累计字节相加。
- 首次观测只建立基线，不回填此前流量。
- 计数器回退会开始新的运行会话；单纯采集缺口不会重置会话。
- 当前阶段尚未进行多维归因，非零全局字节统一记录为 `_unattributed`。

## 本地开发

项目使用 Go 1.26.2。全部可重定向的开发缓存和测试产物保存在 `.flowlens-dev/`。

```bash
make check
```

该命令执行格式检查、`go vet` 和全量 Go 测试。

## 安全边界

- Compose 默认只绑定 `127.0.0.1:8080`。
- FlowLens 不内置 TLS；需要公网访问时，请使用自己的 HTTPS 反向代理。
- 容器以 `10001:10001` 运行，根文件系统只读，移除全部 Linux capabilities。
- FlowLens 不修改 sing-box 配置、路由、防火墙或代理连接。

## License

FlowLens 使用 [MIT License](LICENSE)。
