# AGENTS.md

## 项目定位

FolioChest / 资料匣当前为 `0.6.1-dev` 原型。

阅读顺序：
1. [README.md](README.md) / [README.zh.md](README.zh.md)：定位、构建与使用方式。
2. [docs/DESIGN.md](docs/DESIGN.md)：产品架构与核心模型。
3. [docs/BOUNDARIES.md](docs/BOUNDARIES.md)：安全、数据一致性与恢复约束。
4. [docs/FORMAT.md](docs/FORMAT.md)：磁盘存储与加密封装格式。

## 开发与安全红线

- 默认使用中文回复，直接说明动作、结果和待确认事项。
- 用户要求每次修改应用后完成构建并重启本机预览服务，无需再次询问。重启前核对监听进程、命令行和工作目录，沿用现有设备目录、监听地址及 `--background`；先构建成功再优雅停止旧进程，核对资料库位置与 `vault.json` 指纹不变，不自动打开浏览器。构建或重启失败须如实报告，不宣称已更新。
- 真实资料库、主密码、解锁凭据、MCP 凭据、恢复码、私钥及明文导出不得进入仓库；所有测试只使用合成数据。
- `.local/`、`bin/`、`dist/`、`web/dist/`、`web/node_modules/` 及生成的 Windows `.syso` 不得提交。
- `internal/server/testdata/v1/`、`v2/` 为旧版本生成的合成兼容性样本，不要替换为真实数据。
- 不擅自添加遥测、远程云服务、手机端、浏览器扩展或 Passkey 认证器；应用不内置坚果云或 WebDAV 同步协议。
- `key`、`description` 和单层 `group` 保持明文，`value` 与附件加密；读取与修改按 `key` 应用三态策略，AI 不得修改 `group`。
- 网页与 MCP 共用同一套后端操作与权限逻辑；审批优先留在发起请求的 harness，网页仅在存在待处理请求时显示备用入口。URL 与前端状态不得用作后端授权。
- 历史、恢复、正文搜索、删除、重命名、分组管理、备份、改密码和格式升级仅向已认证网页开放，不开放为免审批 MCP 接口。
- 保留加密 AAD/HMAC 域（`keyfolio/*/v1`）、系统凭据命名空间（`io.github.shenjiecode.keyfolio`）、默认设备目录（`keyfolio/`）、Cookie 与旧启动项标识；唯一命令入口为 `cmd/foliochest`。
- 保留格式 1/2 兼容读写，不静默升级；格式升级必须由人类验证主密码并明确确认 `target_format`。
- 设备解锁凭据只进系统凭据库，不回退到明文文件；改主密码只重新包装原库密钥，不声称数据密钥轮换。
- 网页仅维护 `web/src/main.tsx` 一套正式入口，先运行 `npm --prefix web run build` 再运行全包 Go 测试；保持严格 CSP（Radix/CodeMirror 样式使用每页随机 nonce，不用 `unsafe-inline`）。
- 平台与系统级测试须使用隔离 HOME/XDG、私有 D-Bus 或 mock，不得操作开发者真实凭据、启动项、锁屏、休眠或安装生成的系统包；后台测试应传入 `--background`。
- 默认完整校验命令：`make check`（`npm --prefix web run build` 后执行前端检查/单测、`go vet ./...`、`go test -race ./...`）。
