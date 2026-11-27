<p align="center">
  <img src="packaging/assets/logo.png" alt="FolioChest / 资料匣" width="128" height="128" />
</p>

# FolioChest / 资料匣

[English](README.md) | 简体中文

本地运行、通过网页管理、按 `key` 向 AI 授权的加密资料库。

`key`、`description` 和可选的单层 `group` 保持明文，供 AI 优先检索判断；`value` 与附件使用 AES-256-GCM 加密。外部同步工具（如坚果云等）负责同步文件，资料匣负责加解密、版本历史、冲突检测与按 key 权限控制。

> **当前阶段（`0.6.1-dev`）：** 可运行原型。Windows 真机运行/安装验收和独立安全审计尚未完成，请先使用合成数据并保留独立备份。

## 核心特性

- **本地网页与单文件分发**：Go 后端内嵌 React + TypeScript + shadcn/ui 前端，运行时无需 Node.js 或外部 CDN。
- **明文发现 + 加密正文**：Argon2id（`64 MiB`、3 次迭代）派生解锁密钥，AES-256-GCM 保护库密钥、正文与附件。
- **内容与组织**：支持纯文本、Markdown 安全预览（禁用 HTML 与外链图片）、JSON 辅助编辑、加密附件、单层分组、历史浏览/恢复、分支冲突合并与回收站。
- **按 Key 的 MCP 权限**：每个 key 独立配置读取与修改三态策略（无须授权 / 需要授权 / 拒绝），支持独立客户端凭据、会话内 form elicitation 及网页备用审批。
- **备份与恢复**：完整 ZIP 备份导出，导入前校验密码、认证标签与版本依赖，仅恢复到新目录或空目录。
- **设备集成与自动锁定**：可选系统凭据库自动解锁（Windows Credential Manager / Linux Secret Service）、90 天记住浏览器、用户级登录自启动、网页空闲锁定及系统锁屏/休眠锁定。

## 快速开始（源码运行）

需要 **Go 1.26+** 与 **Node.js 24+**。

```bash
npm --prefix web ci
npm --prefix web run build
go run ./cmd/foliochest
```

访问 `http://127.0.0.1:7463/`，可创建新资料库、打开已有目录或从备份恢复。

构建独立可执行文件：

```bash
make build
./bin/foliochest
```

开发测试时使用隔离的设备配置目录：

```bash
./bin/foliochest --device-dir "$PWD/.local/test-device"
```

### 常用启动参数

- `--device-dir`：本机设备配置目录（不参与同步，默认为系统用户配置目录下的 `keyfolio/`）
- `--vault`：资料库目录（也可在网页中选择）
- `--listen`：回环监听地址（默认 `127.0.0.1:7463`）
- `--background`：仅启动后台服务，不打开浏览器

关闭浏览器标签页不会退出后台，请通过网页中的“退出资料匣”按钮停止服务并锁定资料库。

## MCP 接入

1. 在网页解锁资料库，打开 **设置 → MCP** 创建接入凭据。
2. 客户端连接 `http://127.0.0.1:7463/mcp`，携带请求头 `Authorization: Bearer <接入凭据>`。

| 工具 | 用途 |
| --- | --- |
| `discover_entries` | 检索公开的 `key`、`description` 和 `group`（支持按组精确筛选）；不返回正文 |
| `read_entries` | 按 key 策略申请读取正文或指定附件 |
| `replace_entries` | 基于 `expected_revision` 按修改策略提交完整替换 |
| `request_result` | 查询待审批状态，并单次领取已批准的执行结果 |

以 `pi` 为例（先设置 `FOLIOCHEST_TOKEN` 环境变量）：

```bash
pi mcp add foliochest --url http://127.0.0.1:7463/mcp --bearer-token-env-var FOLIOCHEST_TOKEN
```

## 打包与跨平台构建

生成免安装包（输出至 `dist/`）：

```bash
make package
```

生成免安装包及 Windows NSIS 安装器（x64、实验性 x86）、Linux DEB/RPM（amd64/arm64），需本机安装 **NSIS 3.11** 与 **nFPM 2.49.0**：

```bash
make installers
```

详细工具链与发布流程见 [packaging/README.md](packaging/README.md) 与 [docs/RELEASING.md](docs/RELEASING.md)。

## 测试

```bash
make check
```

依次执行前端类型检查、前端单元测试、`go vet ./...` 和 `go test -race ./...`（直接执行 Go 测试前需先 `npm --prefix web run build`，因为 `web/embed.go` 会嵌入 `web/dist/`）。

## 原型限制

- 单条正文上限 `1 MiB`；每条最多 `16` 个附件，单个附件上限 `2 MiB`。
- 单次批量读写上限 `20` 条、`8 MiB` 序列化载荷。
- 资料库记录总量上限 `64 MiB`、`4096` 个文件（含历史版本）。
- 审批请求有效期 `5` 分钟，结果仅可领取一次。
- 不同 `entry_id` 的同名冲突当前仅拦截并标记冲突，不自动合并。

## 文档

- [项目设计](docs/DESIGN.md)
- [边界与安全说明](docs/BOUNDARIES.md)
- [文件格式](docs/FORMAT.md)
- [安全策略](SECURITY.md)
- [贡献指南](CONTRIBUTING.md)
- [发布流程](docs/RELEASING.md)

## 许可证

采用 [MIT License](LICENSE)。第三方组件与许可证声明保留在 [web/src/components/ui/NOTICE.md](web/src/components/ui/NOTICE.md)，并在打包时由 `tools/package` 一并收集。
