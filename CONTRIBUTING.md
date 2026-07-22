# Contributing to FlowLens

感谢你改进 FlowLens。第一版专注于可靠保存 sing-box Clash API 的流量统计，请让改动保持小而明确，并避免引入与这一目标无关的平台或服务。

## 开发环境

项目固定使用 Go 1.26.2、Node.js 24.14.0 和 pnpm 11.9.0。Makefile 会把可重定向的缓存和测试产物放在仓库内的 `.flowlens-dev/`。

```bash
corepack enable
make deps
make check
make frontend-e2e
```

提交前请确认：

- 示例只使用 RFC 文档地址、`example.test` 或明显不可用的占位值。
- 不包含真实配置、Secret、Cookie、数据库、备份、日志或部署地址。
- 行为变更有对应测试，公开 API 变更同步更新 `api/openapi.yaml` 和 `docs/api-sse.md`。
- Commit 使用 DCO 签名：`git commit -s`。

Pull Request 应说明用户可见变化、验证命令和兼容性影响。请不要把自动生成的依赖目录、构建产物或本机报告加入 Git。
