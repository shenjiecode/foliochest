# 分支与发布

## 分支约定

- `develop`：日常开发与集成分支。
- `main`：发布分支；准备发布时将验证通过的 `develop` 合入 `main`。
- 禁止强制推送或重写已发布的 tag。

```sh
git switch main
git pull --ff-only origin main
git merge --ff-only develop
git push origin main

# 仅在明确发布新版本时打 tag：
git tag -a v0.6.2 -m "FolioChest 0.6.2"
git push origin v0.6.2
git switch develop
```

如分支出现分叉，先在 `develop` 合并 `main` 并解决冲突，再合入 `main`。

## 版本规范

- 正式版本：`vX.Y.Z`
- 预发布版本：`vX.Y.Z-alpha.N`、`vX.Y.Z-beta.N`、`vX.Y.Z-rc.N`
- 向 `main` 推送普通提交不会创建 Release；只有推送符合规则的版本 tag 才会触发 `.github/workflows/release.yml`。

## 自动发布流程

推送版本 tag 后，`.github/workflows/release.yml` 执行以下流程：

1. 校验 tag 格式并确认对应提交属于 `main` 历史。
2. 构建前端与全部 10 个目标产物：Windows x64/x86 的 ZIP 与 NSIS 安装器，Linux amd64/arm64 的 TAR.GZ、DEB 与 RPM。
3. 将去掉 `v` 前缀的版本号通过 `-ldflags` 注入应用版本、状态 API、MCP 服务与安装包元数据（默认开发版本定义在 `internal/buildinfo/version.go`）。
4. 校验 `SHA256SUMS-<version>.txt`，确认 tag 与 `main` 未在构建期间变更后创建草稿 Release，上传全部资产后再公开（预发布标签标为 prerelease；已有同名 Release 拒绝覆盖）。

CI 中 Actions 均固定引用 commit SHA；发布任务仅依赖本次 workflow 的 `GITHUB_TOKEN`。如果上传阶段失败，会保留草稿 Release，排查清理草稿后重新触发即可。
