# Contributing to FlowLens

感谢你改进 FlowLens。第一版专注于可靠保存 sing-box Clash API 的流量统计，请让改动保持小而明确，并避免引入与这一目标无关的平台或服务。

## 开发环境

项目固定使用 Go 1.26.2、Node.js 24.14.0 和 pnpm 11.9.0。Makefile 会把可重定向的缓存和测试产物放在仓库内的 `.flowlens-dev/`。

```bash
corepack enable
make deps
make check
```

## 浏览器按需验收

界面或浏览器行为变更按需使用现有浏览器，通过人工或 computer-use 工具验收。无需项目下载或维护专用浏览器；`make check` 保留格式、类型、单元测试、Go 检查及前端构建，CI 不承担浏览器验收。

- 使用 `pnpm --dir web dev:demo` 检查演示界面的桌面和窄屏布局、主题切换、图表与关键交互；Demo 使用模拟数据，不能代表生产连接。
- 涉及生产行为时，针对本地构建的 FlowLens 服务和脱敏测试数据验收。CSP 需核对实际响应头、页面加载和控制台的策略违规；SSE 需核对网络事件流中的命名事件、重连和界面更新。仅打开页面、截图或 `vite preview` 不能证明这些检查通过。
- 记录使用的浏览器及版本、构建/模式、验收步骤、实际结果，以及必要的响应头、控制台、网络事件或截图证据。不得附带真实凭证或私人业务数据。
- 无法取得相关证据时，明确写“未执行”或“未验证”及原因，不以构建、单元测试或 Demo 通过代替浏览器验收。
- 项目产生的验收截图和临时文件保存在 `.flowlens-dev/`。

提交前请确认：

- 示例只使用 RFC 文档地址、`example.test` 或明显不可用的占位值。
- 不包含真实配置、Secret、Cookie、数据库、备份、日志或部署地址。
- 行为变更有对应测试，公开 API 变更同步更新 `api/openapi.yaml` 和 `docs/api-sse.md`。
- Commit 使用 DCO 签名：`git commit -s`。

Pull Request 应说明用户可见变化、验证命令和兼容性影响。请不要把自动生成的依赖目录、构建产物或本机报告加入 Git。
