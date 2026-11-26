# 分支与发布

## 分支

- `develop`：日常开发与集成分支。
- `main`：准备发布的代码。
- 单人维护阶段不要求 PR、不要求审批、不需要在网页手动合并。由维护者或获授权的编码助手执行直接合并；不自动将每次 develop 提交推到 main。
- 不强制推送或重写已发布 tag。仓库保持私有；当前账号套餐不支持此私有仓库的分支规则，因此这里是工作约定，不宣称服务器已经强制保护 main。
- 两条分支的 push 触发 Build workflow，仅做前端类型检查/构建和跨平台打包，暂不运行回归或启动程序。

## 合并与打标签

工作区干净且构建通过后：

```sh
git switch main
git pull --ff-only origin main
git merge --ff-only develop
git push origin main

# 示例，只有明确要发布该版本时才执行：
git tag -a v0.6.2 -m "FolioChest 0.6.2"
git push origin v0.6.2
git switch develop
```

如两条分支发生分叉，先在 develop 合并 main 并解决冲突，再发布；不使用 reset/force 推送替代合并。

- 正式版本：`vX.Y.Z`。
- 预发布：`vX.Y.Z-alpha.N`、`vX.Y.Z-beta.N`、`vX.Y.Z-rc.N`。
- main 的普通 push 不创建 Release。创建分支、提交 workflow 也不表示已经发布新版本。
- 通过普通 Git 凭据推送 tag 触发。不要假设另一个 workflow 用 GITHUB_TOKEN 推送 tag 会触发后续 workflow。
- 正式/预发布只是版本渠道，不代表已通过安全审计、代码签名或真机验收。

## 自动发布

`.github/workflows/release.yml` 在版本 tag 推送后：

1. 校验 tag 格式，要求其提交已经属于 main 历史。
2. 构建网页和全部十个产物：Windows x64/x86 的 ZIP/安装器，Linux amd64/arm64 的 TAR.GZ/DEB/RPM。
3. tag 去掉 v 后作为统一版本，注入 Go 程序、状态 API、安装包元信息和文件名；开发默认版本仍留在 `internal/buildinfo/version.go`。前端运行时显示状态 API 的版本，不依赖 npm 开发包版本。
4. 校验 SHA-256 并传递不可变 Actions artifact 到发布任务。
5. 发布前再次确认 tag 没有移动、提交仍属于 main。构建任务只有读取权限，仅发布任务持有 contents 写权限。
6. 先创建草稿、上传完整资产，再发布；预发布标签标为 prerelease。已有 Release（包括草稿）拒绝覆盖。

Actions 固定引用 commit SHA。nFPM 固定为 2.49.0，NSIS 从 Ubuntu 构建机的软件源安装；NSIS 的本地验证版本为 3.11，CI 版本由 Ubuntu 软件源决定。

失败时查看 Actions 日志。上传失败会留下草稿，排查后明确删除失败草稿再重跑；不要移动已发布 tag。不能把包构建成功当作安装/升级/卸载验收。

## 权限和当前范围

无需 PAT 或发布密钥，发布使用本次 workflow 的 GITHUB_TOKEN。仓库需允许 Actions；若账号额度、计费状态或平台策略阻止执行，文件配置不会绕过这些限制。

没有配置应用自动更新、代码签名、发布审批或自动公开仓库。统一回归恢复后再将测试加入 workflow。
