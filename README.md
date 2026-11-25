# Keyfolio

本地运行、通过网页管理、按 key 向 AI 授权的加密资料库。

key 和 description 保持明文，value 与附件加密。AI 优先读取 key + description 判断需要哪些条目，再通过 MCP 按权限读取或替换内容。用户通过网页管理资料库，坚果云等外部工具负责同步文件。

**当前阶段：`0.1.0-dev` 可运行原型。已打通网页管理、加密存储与 MCP 读写审批；请先使用合成测试数据。**

## 文档

- [项目设计](docs/DESIGN.md)：产品目标、已确认需求、使用流程、技术方向和实施顺序。
- [边界与安全说明](docs/BOUNDARIES.md)：信任边界、授权约束、加密与恢复、外部文件变化的处理和验收要求。
- [文件格式](docs/FORMAT.md)：当前原型的加密封装、版本记录和恢复依据。
- [开发约定](AGENTS.md)：后续开发及 AI 协作需要遵守的约定。

## 已实现

- 网页创建、选择目录、解锁、锁定，以及条目新增和编辑。
- key + description 明文检索；纯文本、Markdown 源码、JSON 正文与附件。
- Argon2id 派生解锁密钥，AES-256-GCM 保护库密钥和内容。
- 每个 key 的读取/修改三态权限，默认均需确认。
- MCP 元信息检索、按范围读取、批量替换、一次性领取审批结果。
- 网页审批；支持会话式 form elicitation 的 MCP 客户端可在客户端确认。
- 客户端接入凭据的创建与撤销，本机只持久化凭据哈希。
- 不可变批次文件、父版本关系、冲突检测、文件完整性检查和运行时重新扫描。
- 保存前检查版本、保留历史文件；外部变化时保留编辑草稿。

## 待完成

- 系统凭据存储、自动解锁、用户级开机启动和自动锁定设置。
- 完整历史浏览/恢复、冲突合并、条目删除与重命名界面。
- Markdown 渲染预览、正文全文搜索、备份/导出工具和主密码变更。
- 2026-07-28 多轮交互审批与更多 harness 适配；pi 专用扩展尚未实现。
- Windows 真机运行验证及正式发布前的安全复核。

## 产品方向（完整目标）

- Go 后端，TypeScript 网页；网页构建产物嵌入程序。
- 首版支持 Windows 与 Fedora/Linux，按系统和架构分别发布。
- 全部配置和日常使用在本地网页完成，无原生桌面 GUI。
- 一个主密码；每台设备可选择开机启动和自动解锁。
- 明文 key + description、加密 value、加密附件；value 支持 Markdown 等文本内容。
- 每个 key 分别设置读取和修改策略：无须授权、需要授权、拒绝。
- 多个 key 可以一次申请授权；审批优先嵌入 harness。
- 外部同步由坚果云等工具负责，应用处理文件变化、版本和冲突。

## 本地运行

源码构建需要 Go 1.26+ 与 Node.js 20+。仓库指定 Go 1.26.11 工具链；前端仅使用 TypeScript 编译器，无 CDN 或运行时 Node.js 依赖。

```bash
npm --prefix web ci
npm --prefix web run build
go run ./cmd/keyfolio
```

打开 http://127.0.0.1:7463/ 。在网页选择资料库目录，创建资料库或输入已有主密码解锁。

构建独立程序：

```bash
make build
./bin/keyfolio
```

程序也接受显式目录，适合开发时隔离合成数据：

```bash
./bin/keyfolio --vault "$PWD/.local/demo-vault" --device-dir "$PWD/.local/demo-device"
```

默认设备配置位于系统用户配置目录下的 `keyfolio/`，资料库默认位于其中的 `vault/`。可以在网页切换资料库目录。本机接入凭据和会话不放入资料库目录。

## MCP 接入

1. 在网页解锁资料库。
2. 打开“连接与设置”，为 harness 创建独立的接入凭据。
3. 客户端连接 `http://127.0.0.1:7463/mcp`，携带 `Authorization: Bearer <接入凭据>`。
4. AI 优先调用 `discover_entries` 获取 key、description 和版本，再选择所需内容。

工具：

| 工具 | 用途 |
| --- | --- |
| `discover_entries` | 检索公开的 key + description，返回版本与策略，不返回正文 |
| `read_entries` | 按 key 申请正文、指定附件或全部附件 |
| `replace_entries` | 按预期版本替换 value、description 或附件列表，不修改权限 |
| `request_result` | 查询待审批状态，单次领取本客户端的执行结果 |

支持会话式 form elicitation 的客户端可以展示确认框。其他客户端收到 `pending` 后，由用户在网页“待审批”中处理，再调用 `request_result`；主密码不提供给 AI。

例如，在支持该配置的 pi 版本中，可由用户自行设置 `KEYFOLIO_TOKEN` 环境变量后执行：

```bash
pi mcp add keyfolio --url http://127.0.0.1:7463/mcp --bearer-token-env-var KEYFOLIO_TOKEN
```

仓库不会自动修改你的 harness 配置。实际 pi 确认 UI 的端到端接入尚待验证；当前已用官方 Go MCP 客户端验证协议流程。

## 测试与构建

先构建网页，再运行：

```bash
go test -race ./...
go vet ./...
npm --prefix web run check
```

已在本机 Linux 运行自动测试，覆盖加密/篡改、三态权限、批量审批、原生 elicitation、客户端隔离、过期/撤销、版本冲突与同步乱序。浏览器已检查创建、编辑和保存流程。

Windows amd64 与 Linux arm64 可以交叉构建；交叉编译成功不代表已经完成对应系统的运行测试：

```bash
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -o bin/keyfolio-windows-amd64.exe ./cmd/keyfolio
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o bin/keyfolio-linux-arm64 ./cmd/keyfolio
```

## 原型限制

- 正文最多 1 MiB；每个附件最多 2 MiB，每条最多 16 个附件。
- 单个写入批次最多 20 条，序列化后的正文/附件合计最多 8 MiB；单次读取的正文和附件字节合计最多 8 MiB。
- 资料库记录文件总量暂限 64 MiB、4096 个目录项，含保留历史；接近上限时需先保留完整备份。
- 审批请求有效期 5 分钟，结果仅领取一次，重启或锁定会使原请求失效。
- 冲突版本会保留并阻止继续覆盖，当前尚无图形化合并与历史清理功能。
- 实验性格式及更详细的安全边界见 [FORMAT](docs/FORMAT.md) 和 [BOUNDARIES](docs/BOUNDARIES.md)。

Keyfolio 仍是工作名称，目前没有选定开源许可证。
