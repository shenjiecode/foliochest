# 提案：精简防御性注释、过度兼容逻辑与多余复杂度

- 状态：已复核并实施选定范围；原提案保留在 §0–§8，实际采纳范围与验证结果见 [§9](#9-复核与实施结果)。
- 范围：Go 后端、TypeScript 网页、仓库配置、三份文档中的重复免责声明。
- 原则：不改 AGENTS.md 和 BOUNDARIES 里已确认的安全/一致性语义；只删"重复说文档的话"和"为不存在的旧客户端准备的分支"。

## 0. 结论

| 类别 | 数量 | 风险 | 需要你决定 |
| --- | --- | --- | --- |
| A. 直接可做 | 10 项 | 低 | 否 |
| B. 值得做，改动稍大 | 3 项 | 中 | 否 |
| C. 涉及已确认决策或文档承诺 | 4 项 | — | 是（见 §7） |
| D. 检查过但建议保留 | 12 项 | — | 知悉即可 |

预计净减少约 250–300 行（含测试与文档），不减少任何测试覆盖的行为。

## 1. 防御性注释（Go）

现有 118 行 Go 注释里，大部分是解释"为什么"的好注释（如双时钟比较、批次就绪判定、logind 回退路径），建议保留。下面是只复述文档、声明"代码不做某事"、或全大写强调的部分：

| 位置 | 现状 | 处理 |
| --- | --- | --- |
| `internal/access/service.go:122` | `It is deliberately NOT exposed as an MCP tool.` | 删 |
| `internal/vault/groups.go:106-107` | `There is no permanent catalog; a concurrent offline writer can make the old name reappear.` | 删第 2 句，保留"Empty target removes the group without deleting entries." |
| `internal/vault/manage.go:21` | `it does not erase encrypted history or free its current name for a new entry.` | 删；代码本身就是 `m.Deleted = true` + 新批次 |
| `internal/vault/history.go:32` | `..., never MCP tools.` | 删后半句，保留 human-only 契约 |
| `internal/vault/history.go:123` | `It never erases history or restores an older, more permissive policy.` | 删；`restrictive()` 已说明 |
| `internal/vault/device.go:122` | `Old backups and copied data keys remain usable. It does not claim to revoke...` | 删，保留 `rewraps the same data key` |
| `internal/vault/device.go:142-143` | 整段"explicitly confirmed by a human after backing up...does not silently migrate" | 改为一句：`// Rewraps the same key with format = FormatVersion; record files are not rewritten.` |
| `internal/vault/backup.go:91` | `Existing data is never overwritten. Only Keyfolio's archive layout is accepted; no general extraction.` | 删（还带着旧品牌名） |
| `internal/platform/system.go:3` | `..., never credentials or content.` | 删后半句 |
| `internal/platform/system_windows.go:59` | `We use no account names from the rest of the native structure.` | 删 |
| `internal/platform/system_windows.go:69-70` | Windows 7 反转标志、不在支持范围 | 改为 `// Windows 10/11 semantics.` |
| `internal/platform/system_windows.go:186` | `Never call ShowWindow; there is no desktop UI or taskbar window.` | 删 |
| `internal/server/system.go:77` | `Native observers don't register startup entries, touch credentials or change the OS lock state.` | 删 |
| `internal/server/device.go:127` | `and never grants authentication to a new browser just because the vault is open.` | 删后半句（`authenticateTrusted` 的注释已覆盖） |
| `cmd/keyfolio/main.go:1`、`internal/app/run.go:1` | "legacy ... alias" | 随 §7 决策 1 一起处理 |
| 全大写强调 | `index.go:6` EVERY、`groups.go:31` EVERY、`history.go:122` ALL、`device.go:69` ONLY、`device.go:121` SAME、`platform/system.go:5` NOT | 改小写 |

## 2. 过度兼容逻辑

| # | 位置 | 现状 | 建议 | 理由 |
| --- | --- | --- | --- | --- |
| 2.1 | `internal/server/http.go:116-118` | CSRF 头缺失时回退读 `X-Keyfolio-CSRF` | **删除**；`http_test.go:72` 改用新头 | 网页资源嵌入同一程序且 `Cache-Control: no-store`，不存在会长期发送旧头的客户端。唯一场景是"旧程序开着的标签页没刷新就换了新程序"，此时返回的 403 文案已是"请刷新网页"。 |
| 2.2 | `internal/platform/startup_linux.go:56-58` + `startup_linux_test.go:36-41` | 读取启动项时把 `Name=Keyfolio` 替换成 `Name=FolioChest` 再比较 | **删除**该分支和测试 | 旧条目会显示"启动项与当前程序位置不一致，请重新保存"，用户点一次保存即可，无数据风险。 |
| 2.3 | `cmd/keyfolio/`、`Makefile` 的 `bin/keyfolio`、README "兼容入口" | 同时构建两个等价二进制 | 见 §7 决策 1 | 这是 DESIGN §4.5 曾确认的保留项；但它是 0.x 原型、仅用合成数据，且 README 已说明"更换可执行文件路径后需重新保存启动项"。 |
| 2.4 | `internal/vault/store.go:225` | `b.Format == 1 && p.meta.Key != m.Key` 把批次判为未就绪 | 可删（微小） | 新程序在格式 1 下拒绝改名（`ErrUpgrade`），不可能写出改 key 的格式 1 批次；此分支只对手工构造文件生效。 |
| 2.5 | 默认设备目录 `…/keyfolio/` | 保留 | **保留** | 只是一个字串，不是复杂度；改会让现有 `clients.json`/`device.json` 失联。 |
| 2.6 | 凭据命名空间 `io.github.shenjiecode.keyfolio`、Cookie 名 `kf_*`、注册表值名 `Keyfolio-<id>`、Desktop 标记 `X-Keyfolio-Device`、AAD 域 `keyfolio/*/v1` | 保留 | **必须保留** | 改名会使已存的库密钥凭据、启动项、旧批次不可读。 |
| 2.7 | 格式 1/2 读写、`omitempty`、各处 `cfg.Format < 2/3` 门槛、`testdata/v1`、`testdata/v2` | 保留 | **必须保留** | AGENTS.md 硬约束；且有真实旧版本生成的样本测试。 |

## 3. 不必要的复杂度

### 3.1 前端（`web/src`）

| # | 现状 | 建议 | 收益 |
| --- | --- | --- | --- |
| 3.1.1 | `main.ts`、`manage.ts`、`history.ts`、`device.ts` 各有一份"提交时禁用全部控件、结束后恢复"的代码（`prior = [...].map(el => [el, el.disabled])` 共 4 份） | 抽成 `ui.ts` 的 `withBusy(form, fn)` | -40 行，4 处行为一致 |
| 3.1.2 | `main.ts:681` 重复 `ui.ts` 的 `base64()`；`main.ts:716` 与 `history.ts:155` 重复 `atob → Uint8Array → Blob → download` | 删 `main.ts` 内联版本，`ui.ts` 增加 `downloadAttachment(att)` | -15 行 |
| 3.1.3 | 四个代次计数器 `sessionEpoch`/`entryLoadSequence`/`entriesSequence`/`searchSequence`，35 处引用，每处 handler 手写 `if (!x.isConnected \|\| sequence !== n) return;` | 合并为一个 `viewGeneration`（导航/切会话/切条目时 +1），`api()` 接收 `AbortSignal`，请求过期时由 `fetch` 自己抛 `AbortError`。`GroupUI` 的 `revision` 回调随之删除 | -1 个概念，-20 余处判断；现有 groups.test 的"异步结果不覆盖新草稿"测试继续有效 |

### 3.2 后端（Go）

| # | 现状 | 建议 | 收益 |
| --- | --- | --- | --- |
| 3.2.1 | `vault` 包 18 个导出方法开头都是同样 5 行：`if len(v.key)==0 {return ErrLocked}; if err := v.scan(); err != nil {return err}` | 一个 `func (v *Vault) fresh() error`（需在持锁下调用） | -50 行 |
| 3.2.2 | "写临时文件 + chmod 0600 + Sync + Rename + 目录 Sync" 有 4 份：`vault/crypto.go publish`、`vault/device.go replaceConfig`、`server/clients.go writeLocal`、`platform/startup_linux.go Set`；`readBounded` 与 `server/device.go loadDevice` 的限长读取也重复 | 新建 `internal/fsutil`：`WriteFile(path, data)`（覆盖）、`Publish(dir, name, data)`（拒绝覆盖）、`ReadBounded(path, max)` | -60 行，一处维护 fsync 语义 |
| 3.2.3 | `Vault.List/Trash/Search(query string, group ...string)` 用可变参数模拟可选过滤；`http.go` 再按 `scope=trash\|body` 分三路；`mcp.go` 把 `*string` 转成 `[]string` 再展开 | 改为 `List(ListOptions{Query, Group *string, Body, Trash bool})`，删 `Trash`/`Search` | -2 个导出方法，去掉可变参数技巧；`mcp.go` 直接传 `args.Group` |
| 3.2.4 | `time.Now().UTC().Format(time.RFC3339Nano)` 5 处 | `func stamp() string` | -4 行 |
| 3.2.5 | `in.Password = ""`、`defer func(){ in.Current = ""; in.Next = "" }()` 等 5 处字符串置空 | 删除 | Go 字符串不可变，置空不擦除任何副本；BOUNDARIES §6 自己也写了"不能用变量赋空代替承诺"。`clear([]byte)` 保留。 |
| 3.2.6 | `platform/startup_linux.go desktopQuote` 对 `\`、`$`、`` ` `` 做转义，但上面第 27–30 行已经拒绝含这些字符的参数 | 删这三个 Replacer 分支 | 死代码；`%`、`"` 分支保留 |
| 3.2.7 | `access.Service` 的 32 MiB `bufferedBytes` 计量：1 个字段、13 处读写 | 见 §7 决策 3 | 只保留 32 个请求上限的话最坏 32×8 MiB，对单用户本机程序可接受；但 BOUNDARIES 明确写了 32 MiB。 |

### 3.3 仓库

| # | 现状 | 建议 |
| --- | --- | --- |
| 3.3.1 | `.gitignore` 20 条，其中 `/data/`、`/vaults/`、`/backups/`、`/exports/`、`.env*`、`*.pem`、`*.key`、`*.p12`、`*.kdbx`、`/dist/`、`/node_modules/` 本项目从不产生（规则要求资料库不进仓库，程序也不写这些路径） | 精简为 `.local/`、`bin/`、`web/dist/`、`web/node_modules/`、`.cynos/` |

## 4. 文档中的重复免责声明

| 文件 | 现状 | 建议 |
| --- | --- | --- |
| `README.md` §测试与构建 | `0.3.0-dev`/`0.4.0-dev`/`0.5.0-dev` 三段，每段末尾各一句"不能替代真机验收 / 未经真实资料验收"，顶部"当前阶段"已说过一次 | 合并为一段"测试覆盖"清单 + 一句免责 |
| `README.md` §设备授权、自启动与主密码 | 失败回退细节（"启动还要求监测就绪、系统会话未锁定且本次启动尚未收到锁定事件"等）与 BOUNDARIES §2.5 近乎逐句重复 | README 只留用户可见行为和操作步骤，边界细节链接到 BOUNDARIES（符合 AGENTS.md"防御性细节集中维护在 BOUNDARIES"） |
| `README.md:155`、`:177` | "不阻止休眠，不保证通知零延迟或休眠前彻底擦除内存"；"交叉编译成功不代表已经完成对应系统的运行测试" | 前者删（BOUNDARIES §2.5 有），后者保留一次 |
| `docs/DESIGN.md` §14 | 四段近乎相同的"Extension 模式"描述，五项取向分数重复 4 次 | 合并为一段"0.2–0.5 均采用 Extension 模式……"+ 各版本新增内容的 bullet |
| `docs/BOUNDARIES.md` 首节"当前原型的实现状态" | 与 README"已实现"重复列功能 | 删功能列表，只保留约束与"尚未提供"清单 |

## 5. 网页文案中的重复警告（可选）

同一条警告在"说明文字 + 行内警告 + 确认框 + 成功提示"里出现 2–5 次。建议每条只保留一处（优先确认框或行内警告）。涉及测试断言的地方需同步改测试。

| 警告 | 出现位置 | 保留 | 删除 |
| --- | --- | --- | --- |
| 不是安全擦除 | `manage.ts:13`、`manage.ts:35`、`history.ts:91`、`main.ts:316` | `manage.ts:13`（有测试）、`main.ts:316` | `history.ts:91` 确认框里的"这不是安全擦除" |
| 旧备份仍用旧密码 / 已复制密钥不可撤销 | `device.ts:19`、`device.ts:141` | `device.ts:19`（有测试） | `device.ts:141` 确认框后半句 |
| 旧程序不能打开格式 3 | `manage.ts:71`、`manage.ts:72` | `manage.ts:72`（有测试） | `manage.ts:71` 卡片里的第二句 |
| 不覆盖现有资料 | `recovery.ts:3`、`recovery.ts:14` | `recovery.ts:3` | `recovery.ts:14` 确认框里的"不覆盖原目录" |
| 历史保留 / 不删除历史 | `history.ts` 内 5 处 | 标题说明、确认框 | 说明段"历史不会被删除"、成功提示"原有历史与附件仍然保留" |
| 不承诺休眠前彻底擦除内存 | `main.ts:773` 设置页 | — | 整句，留"监测中断或进程停顿超过 15 秒也会锁定" |
| AI 不能取回已删除正文 | `main.ts:316` 回收站 caption | — | 后半句 |

## 6. 检查过但建议保留的"防御性"代码

这些都是有明确理由、或 AGENTS.md/BOUNDARIES 已确认的约束，删了会改变行为或安全性：

1. `index.go:45-62` 已观察 head 消失时标记 missing，不回退祖先 —— BOUNDARIES §3/§7.6 明确要求；代码只有 18 行。
2. `access.expire`、`server.session`、`ExpireIdle` 的单调/墙上双时钟取先到期 —— 休眠后单调钟不走，只用单调会延长审批/会话有效期。
3. `platform/credentials_linux.go connectLocalBus` 手工解析 `unix:` 地址并限时拨号 —— godbus 的 `SessionBus()` 无法限制超时，也无法拒绝非 Unix 传输。
4. `system_linux.go:189` 校验信号 sender —— D-Bus 定向信号绕过 match rule，这是实际的伪装向量。
5. `server/device.go disableDevice` 先落盘停用标记再删系统凭据 —— AGENTS.md 规则。
6. `beginUnlock/finishUnlock` 代次校验 —— 防慢 KDF 跨越锁定事件完成解锁。
7. `decodeStrict`/`DisallowUnknownFields`/尾部内容检查 —— 格式严格性，防止旧读取器误读。
8. `readBounded` 的 Lstat + 大小预检 + LimitReader 三重检查 —— 分别拒绝符号链接、避免白读大文件、防止 stat 后增长。
9. `system_other.go`/`credentials_other.go`/`startup_other.go` 桩 —— 让 `go vet ./...` 在任意开发机可跑。
10. `mcp.go maybeElicit` 的协议版本和 URL 模式判断 —— 对应 README 待办的 2026-07-28 多轮审批，是回退到网页的安全阀。
11. `Vault.Unlock` 在已解锁且配置未变时只重扫 —— 第二个浏览器输入主密码后加入同一解锁代次，有测试。
12. `maintenance.TryLock` → 409 —— 并发写返回明确错误而不是排队，5 行。

## 7. 需要你确认的决策

1. **`cmd/keyfolio` 别名二进制**：删除（只保留 `foliochest`）？还是按 DESIGN §4.5 继续保留？建议删除；Go module 路径与默认设备目录不动。
2. **§4 文档精简范围**：全部做，还是只做 README 测试段落和 DESIGN §14？
3. **§3.2.7 审批缓冲 32 MiB 计量**：删除只留 32 请求上限（需改 BOUNDARIES 一句），还是保留？
4. **§5 网页文案去重**：做（会改 4 个测试断言附近的内容），还是先不动 UI？

## 8. 实施顺序

每步一个提交，便于回退：

1. Go 注释清理（§1）+ 字符串置空删除（§3.2.5）+ `.gitignore`（§3.3.1）。
2. CSRF 旧头（§2.1）+ Linux 启动项旧名（§2.2）+ 对应测试与 README/BOUNDARIES 各一句。
3. `fsutil` 抽取（§3.2.2）、`fresh()`（§3.2.1）、`stamp()`（§3.2.4）、`desktopQuote`（§3.2.6）。
4. `List(ListOptions)`（§3.2.3）。
5. 前端 `withBusy`/`downloadAttachment`（§3.1.1–3.1.2）。
6. 前端代次合并（§3.1.3）—— 最容易出错的一步，单独提交，依赖现有 groups.test。
7. 文档与文案（§4、§5）按决策执行。

每步后运行：`npm --prefix web ci && npm --prefix web run build && go test -race ./... && go vet ./... && npm --prefix web run check && npm --prefix web test`，以及 Windows amd64 / Linux arm64 交叉编译。

## 9. 前次精简的复核与实施结果

**0.6.0-dev 后续决定：**按用户后续清理要求，正式界面已迁入 React 并连接真实 API；旧 DOM、独立预览、内存示例、`cmd/keyfolio` 和 `X-Keyfolio-CSRF` 入口均已删除，不保留前端归档副本。原 `ui.ts` 工具由 `web/src/lib/hooks.tsx`、`web/src/lib/files.ts` 及对应回归测试接替。以下“保留旧头/命令”和旧前端文件说明只记录前次结果，不再代表当前实现。资料格式、加密域、凭据/Cookie/启动项标识及 Linux 启动项显示名识别不变；当前界面以 DESIGN §14 为准。

以上删行估算是原提案，不作为交付目标。前次精简优先消除重复实现，保留公开 HTTP/MCP 接口、安全约束和测试；新增回归测试，不用删除测试换取净行数。

### 已采纳

- §1：精简复述文档的注释、降低全大写强调；保留锁持有条件、授权调用契约、同步依赖和操作顺序的理由。
- §3.2.1、§3.2.4–6：抽取持锁下的 `fresh()`、统一 `stamp()`，去掉 Go 请求结构中无擦除保证的密码字符串置空及已被上游拒绝的 Desktop 转义分支。密钥字节清理、浏览器密码输入清理和对外响应脱敏均保留。
- §3.1.1–2：`ui.ts` 提供 `withBusy`、`encodeBase64`、`downloadAttachment`。保留设备授权失败后的重新读取、历史加载代次失效、原先已禁用的控件状态、密码输入清理和异步结果检查。
- §4：README 按测试主题合并验证记录、设备设置只保留操作与用户可见行为，边界链接到 BOUNDARIES；DESIGN §14 合并 Extension 说明；BOUNDARIES 开头删除已实现功能枚举，后续约束保留。
- §5：只删除历史表单和保存成功提示中的重复“保留历史”说明；删除、改密码和升级操作旁的警告保留。不同操作入口的提醒不一律当作重复。

### 调整后采纳

- §3.2.2：新增 `internal/fsutil`，共用限长读取、临时文件写入/刷新、设备本地文件替换及尽力刷新目录。资料库的 `publish` 仍检查目标是否已存在；`replaceConfig` 仍在临时写入后比较旧配置、替换后锁定并核对结果，不增加先删旧文件的回退。未抽取声称跨平台原子拒绝覆盖的通用 `Publish`：现有检查与 rename 之间仍不是跨进程事务。
- §3.2.3：统一为 `List(ListOptions)`，使用互斥的 `ListScope`，避免 Body/Trash 两个布尔值产生无意义组合。零值仍只查公开元信息；精确组过滤用 `*string` 区分全部与未分组。HTTP/MCP 参数不变，MCP 不传入正文/回收站范围。
- 附件上传仅共用同步编码函数；在 `arrayBuffer()` 返回后，仍先检查会话、条目代次及输入节点存活，过期时清理字节，不直接改成无检查的 `await base64(file)`。

### 本轮不采用

- §2.1–3、§7.1：保留旧 CSRF 头、Linux 启动项显示名和 `cmd/keyfolio` 入口。兼容逻辑很短，且已经写入既有约定；“嵌入资源 + no-store”不能使仍打开的旧标签页或手动 HTTP 客户端消失。
- §2.4：保留格式 1 的父子 key 校验。读取外部记录必须验证其格式语义，不能从“当前程序不会写出这种记录”推导为可省略；新增有合法认证但含格式 1 改名的回归样本。
- §3.1.3：保留会话、列表、搜索、条目的独立代次。列表轮询不能使编辑操作失效；fetch 取消不撤销已提交的服务端写入，也不覆盖 `File.arrayBuffer()` 等非 fetch 异步操作。现有 groups 测试不足以证明把这些生命周期合为一个是等价的。
- §3.2.7、§7.3：保留 32 MiB 内容缓冲与 32 请求记录双上限；删去前者会改变内存和敏感内容驻留上限。
- §3.3.1：保留 `.gitignore` 防误提交规则。用户手动拷入的资料、导出和密钥同样需要防护，不取决于程序是否主动生成这些文件。
- §6 的十二项保留建议继续有效；凭据命名空间、Cookie、启动项标识、加密域及格式 1/2 兼容均不变。

### 验证

- `make check`（前端构建、类型检查及单测、Go vet/race）通过。
- Windows amd64 / Linux arm64 交叉构建、Windows vet，以及新文件工具/资料库/HTTP 测试的 Windows 编译通过；不等于 Windows 真机运行。
- 新构建的独立程序通过格式 2 旧写入兼容、格式 3 显式升级、分组/回收站/ZIP 往返，以及旧程序拒绝新配置和先到达新记录的冒烟测试；私有 D-Bus 的 Secret Service/logind 集成测试实际 PASS，未跳过。
- 新增回归覆盖限长/符号链接读取、临时文件清理、替换失败保留目标、不可变记录与配置比较、查询范围隔离、格式 1 父子 key，以及公共表单状态恢复和附件编解码/下载。
- 未改数据格式、权限策略、公开 HTTP/MCP 请求结构、容量上限或前端异步代次。
