# 原型文件格式

适用于 `0.1.0-dev`、`0.2.0-dev` 与当前 `0.3.0-dev`；资料库磁盘格式未改变，新增设备文件不参与同步。格式号为 1，仍属实验性格式，正式发布前需要复核与迁移方案。

本文件说明实际实现，产品目标见 [DESIGN](DESIGN.md)，边界与尚未完成的保护见 [BOUNDARIES](BOUNDARIES.md)。

## 1. 文件布局

```text
资料库目录/
├── vault.json
└── records/
    ├── <batch-id>.json
    └── <batch-id>.json
```

- 每次保存产生一个新的批次文件，旧文件保留。
- 一个批次可以包含多个条目的变更。
- 文件名使用随机批次 ID，key 和 description 则以明文写在批次头中。这是存储标识与逻辑名称分离，不是隐藏 key。
- 同一批次的正文与附件合并为一个加密 payload，避免正文与附件分文件到达造成半批可用。
- 临时文件使用 `.pending-*`，不作为正式记录读取。
- 本机客户端凭据、会话及网页状态存放在资料库目录外。

## 2. 库配置与主密码

`vault.json` 字段顺序对应 Go `config` 结构：

1. `format`：1。
2. `vault_id`：16 个随机字节的 32 位十六进制字符串。
3. `kdf`：`argon2id`。
4. `salt`：16 个随机字节，JSON 中为 Base64。
5. `memory_kib`：65536，即 64 MiB。
6. `iterations`：3。
7. `parallelism`：1。
8. `wrapped_key`：经过 AES-256-GCM 加密的 32 字节随机库密钥，JSON 中为 Base64。

主密码按 UTF-8 字节输入 Argon2id，输出 32 字节密钥。当前参数固定，读取配置时先检查参数范围与格式，再执行 KDF。

库密钥的加密附加认证数据（AAD）为：

```text
UTF8("keyfolio/config/v1\x00") || JSON(config with wrapped_key = null)
```

这里的 `\x00` 表示一个零字节，不是四个文本字符。

AES-GCM 输出按 `12 字节随机 nonce || ciphertext || 16 字节认证标签` 保存。32 字节库密钥对应的整个封装为 60 字节。

AAD 中的 JSON 使用 Go 标准库 `encoding/json.Marshal`，按上述结构字段顺序、不加缩进生成。字节数组使用 Base64，遵循该编码器对字符串的转义规则；这不是 RFC 8785 的通用规范化 JSON。

### 2.1 变更主密码

验证旧密码及当前快照后，以新密码、新的 16 字节 salt、新 nonce 重新加密原有 32 字节库密钥；库 ID 不变。临时配置写完并刷新后，在同一目录替换 `vault.json`，保留全部记录文件。成功后结束解锁会话。旧备份中的 `vault.json` 仍可用旧密码解开；这不是数据密钥轮换。

### 2.2 本机设备凭据（不属于同步格式）

`device.json` 位于设备配置目录，保存启用状态、规范化资料库目录、`binding.vault_id`、`binding.config_hash`、随机 `credential_id`、可选浏览器 Cookie 的 SHA-256 哈希及到期时间。它不包含主密码或库密钥；禁止放在资料库目录内。

系统凭据库中的载荷为版本 1 JSON：`version, vault_id, config_hash, key, proof`。`config_hash` 是完整 `vault.json` 文件字节的 SHA-256 小写十六进制；`key` 是原库密钥的 Base64，`proof` 是下式 HMAC 的 Base64：

```text
HMAC-SHA256(key, UTF8("keyfolio/device/v1\x00" + vault_id + "\x00" + config_hash))
```

这里 `\x00` 同样表示零字节。该 HMAC 用来检测凭据损坏及绑定不一致，不是对持有库密钥者的撤销机制。应用还比较本机保存的绑定和路径；不把载荷作为公开 HTTP/MCP 参数接收。

系统凭据命名空间为 `io.github.shenjiecode.keyfolio`，每次授权使用新的随机凭据标识。Linux 的 Secret Service 查找属性为 `application` 与 `credential`；Windows 目标名称为 `<命名空间>/<credential_id>`。停用标记先落盘，再尝试删除凭据；失败保留标识供清理。

浏览器另持有 `kf_trusted_<设备目录标识>` Cookie，默认不开启，有效期 90 天；服务端只存哈希。空闲设置仍在 `settings.json`，当前目录在 `location.json`，MCP 客户端凭据哈希在 `clients.json`。这些文件和系统凭据均不进入备份 ZIP。

## 3. 批次记录

批次文件依次包含：

- `format`。
- `vault_id`。
- `batch_id`。
- `changes`：本次变更的元信息数组。
- `payload`：加密正文数组，JSON 中为 Base64。

每项 change 的字段顺序为：

```text
entry_id, revision, parents, key, description,
read_policy, write_policy, updated_at
```

- `entry_id`：创建条目时生成，后续版本沿用。
- `revision`：每次变更生成新的 16 字节随机 ID。
- `parents`：本次变更基于的版本 ID 数组。创建时为空数组。
- `key`、`description`：明文发现信息。
- `read_policy`、`write_policy`：`allow` / `ask` / `deny`。
- `updated_at`：UTC RFC3339Nano 时间；不用于决定冲突胜负。

权限、版本、ID 和时间目前也在明文头中。整个头通过 AAD 获得完整性保护。

批次 AAD：

```text
UTF8("keyfolio/batch/v1\x00") || JSON(batchHeader)
```

`batchHeader` 仅包含前四项，即 `format, vault_id, batch_id, changes`，使用相同的 Go JSON 序列化规则。正文由库密钥进行 AES-256-GCM 加密，使用新的随机 nonce；封装顺序同上。

解密后的 payload 是与 changes 等长、顺序对应的文档数组，每项为：

```text
value, format, attachments
```

每个附件包含 `name, media_type, data`；data 在内部 JSON 中为 Base64。这一层 JSON 整体加密，因此附件名称和媒体类型不会出现在批次明文头中。

## 4. 校验与当前版本

接纳批次前，程序检查：

1. 格式、库 ID、批次 ID 和条目 ID。
2. 元信息结构、策略取值和字段上限。
3. AES-GCM 认证及解密。
4. changes 与文档数组一一对应。
5. 文档和附件类型、数量与大小。
6. 所有条目的父版本依赖已到齐，并属于正确的条目。

父版本依赖按整个批次判断。一条变更缺少依赖时，该批次的其他变更也不会作为可用内容提供。

对同一 entry_id，不被任何其他版本列为父版本的记录是候选 head：

- 一个有效 head：当前版本。
- 多个 head：并发冲突，保留所有记录并暂停该条目读写。
- 依赖缺失：未完整同步状态。
- 当前进程已观察到的 head 消失：缺失状态，不回退成旧内容继续使用。

最后一项的观察状态目前只在内存中；重启或锁定后没有持久化防回退保证。

当前通过每两秒扫描，以及读取/提交前重新扫描发现文件变化，尚未使用文件监听 API。容量检查和文件类型检查见代码与 README。

## 5. 提交

1. 验证本次所有 key、当前版本和权限。
2. 构造完整的元信息与文档数组。
3. 校验并加密。
4. 写临时文件，刷新文件内容。
5. 在同一目录改名为新批次文件。
6. 重新扫描确认当前可用视图。

这是一份应用层单文件批次，不依赖同步工具提供事务。其他电脑收到不完整文件时等待完整校验通过。

加密验证由受信任后台完成。后台可以解密整个批次，随后按 key 和附件范围筛选返回给 AI 的内容；部分授权由服务端执行，不是为每个 key 向 AI 发放独立解密密钥。

## 6. 备份与恢复依据

备份应完整复制 `vault.json` 与 `records/`，保留历史依赖。文件同步进行中取得的副本可能尚未完整，应在新位置验证能够解锁和读取。

当前程序支持完整目录复制，也可通过网页导出 ZIP、导入新目录。ZIP 只包含 `vault.json` 与 `records/<batch-id>.json`，不另行加密 ZIP 容器，因此元数据仍可见；正文与附件继续保持原密文。导入先在隔离目录验证密码、认证和依赖，不覆盖非空目标。

历史恢复与同一条目的正文合并都生成新批次，其 parents 包含全部已确认的当前 head。恢复不重放旧权限；合并逐项采用当前分支中更严格的读取/修改策略。附件沿用所选版本，其他版本仍保留。

专门的离线 CLI、明文导出、历史清理、不同条目同名冲突处理及完整的数据密钥轮换尚未完成。

客户端凭据不随资料库恢复；新设备在网页重新创建自己的客户端接入关系。格式的实现参考位于 `internal/vault/crypto.go`、`internal/vault/store.go`、`internal/vault/device.go`、`internal/platform/` 与对应测试。
