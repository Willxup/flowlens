# FlowLens 运维指南

本文档集中说明首次部署、健康检查、备份恢复、升级和常见故障。配置项本身见 [`config/config.example.yaml`](../config/config.example.yaml)。

## 首次部署

FlowLens 需要 Docker、Docker Compose、已启用 Clash API 的 sing-box，以及两者共同加入的 Docker 用户自定义网络。

```bash
cp config/config.example.yaml config/config.yaml
mkdir -p data
docker network create flowlens_private
```

编辑 `config/config.yaml`：

- 将 `clash_api.url` 和 `clash_api.secret` 设置为 FlowLens 可访问的 sing-box Clash API 地址与 Secret。
- 默认保持 `auth.enabled: true`，并为 `auth.access_key` 设置至少 16 个字符的随机密钥。
- 按部署位置设置 `time.timezone`；数据库首次写入后不得修改。
- 根据需要检查保留期、隐私模式、存储软上限和备份策略。

Linux 主机需要让固定容器用户读取配置并写入数据目录：

```bash
sudo chown 10001:10001 config/config.yaml data
chmod 600 config/config.yaml
chmod 700 data
```

让 sing-box 加入 `flowlens_private`，然后启动 FlowLens：

```bash
docker compose -f docker-compose.example.yml pull
docker compose -f docker-compose.example.yml up -d
```

默认地址为 [http://127.0.0.1:8080](http://127.0.0.1:8080)。示例 Compose 不会公开 Clash API，也不会把 FlowLens Web 暴露到宿主机以外。

## 健康检查

容器自身带有 HEALTHCHECK，也可以手动检查 Web 与 API：

```bash
docker compose -f docker-compose.example.yml exec flowlens /flowlens healthcheck
```

`doctor` 是离线检查。它会取得数据目录锁，只读检查配置、已有 SQLite 数据库及必要的 Clash API 能力，不会创建数据库或执行迁移。

```bash
docker compose -f docker-compose.example.yml stop flowlens
docker compose -f docker-compose.example.yml run --rm flowlens doctor
docker compose -f docker-compose.example.yml up -d
```

## 备份

FlowLens 会按 `backup.local_time` 自动创建经过校验的本地备份，并按 `daily_keep` 和 `monthly_keep` 保留。备份只保存在配置的数据目录中，不会自动外传。

手动备份需要独占数据目录：

```bash
docker compose -f docker-compose.example.yml stop flowlens
docker compose -f docker-compose.example.yml run --rm flowlens backup
docker compose -f docker-compose.example.yml up -d
```

## 恢复

恢复前停止服务，并先校验备份清单。清单路径必须使用容器内绝对路径。

```bash
docker compose -f docker-compose.example.yml stop flowlens
docker compose -f docker-compose.example.yml run --rm flowlens \
  restore --check /var/lib/flowlens/backups/flowlens-YYYYMMDDTHHMMSSZ.manifest.json
docker compose -f docker-compose.example.yml run --rm flowlens \
  restore --output /var/lib/flowlens/restored.db \
  /var/lib/flowlens/backups/flowlens-YYYYMMDDTHHMMSSZ.manifest.json
```

`restore --output` 只创建不存在的新数据库，绝不会覆盖活动数据库。确认恢复文件后：

1. 在宿主机备份现有 `data/flowlens.db*`。
2. 将 `data/restored.db` 原子替换为 `data/flowlens.db`。
3. 启动服务并再次运行 `doctor`。

## 升级

升级前建议先创建一次手动备份，然后拉取目标镜像：

```bash
FLOWLENS_IMAGE=ghcr.io/willxup/flowlens:vX.Y.Z \
  docker compose -f docker-compose.example.yml pull
FLOWLENS_IMAGE=ghcr.io/willxup/flowlens:vX.Y.Z \
  docker compose -f docker-compose.example.yml up -d
```

需要迁移数据库时，FlowLens 会先创建升级前快照。不要跳过备份，也不要让两个 FlowLens 实例共享同一个数据目录。

## 远程访问

- 示例 Compose 默认发布为 `127.0.0.1:8080:8080`。
- 远程访问应通过可信反向代理提供 HTTPS。
- `auth.enabled: false` 会开放页面和全部业务 API，只能用于可信局域网。
- FlowLens 不内置 TLS，也不会修改 sing-box 或主机网络配置。

## 常见故障

| 现象                  | 检查项                                              |
| --------------------- | --------------------------------------------------- |
| `healthcheck` 失败    | 容器状态、`server.listen` 与 Compose 容器端口       |
| `doctor` storage 失败 | 数据卷权限、数据库是否存在、是否有第二个实例占锁    |
| Clash API 检查失败    | Docker 网络、服务名、端口、Secret 与 Clash API 开关 |
| 页面显示采集降级      | 数据质量事件、采集缺口与 sing-box API 可用性        |
| 达到存储软上限        | 保留期、`storage.soft_limit` 与容量保护状态         |

如需报告安全问题，请按 [`SECURITY.md`](../SECURITY.md) 私下联系。
