# 分发构建

## 构建机工具

- 项目构建：Go 1.26+、Node.js 24+（最终用户运行产物时不需要安装 Go 或 Node.js）。
- Linux 安装包（DEB / RPM）：[nFPM](https://nfpm.goreleaser.com/) `2.49.0`：
  ```sh
  go install github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.49.0
  ```
- Windows 安装向导：[NSIS](https://nsis.sourceforge.io/) `3.11`（Debian/Ubuntu 可安装 `nsis`，Fedora 可安装 `mingw32-nsis`；上游声明保存在 `packaging/windows/NSIS-LICENSE.txt`）。
- 可通过环境变量 `NFPM`、`MAKENSIS`、`NSISDIR` 指定自定义工具路径。

## 打包命令

```sh
make package       # 生成 Windows amd64 与 Linux amd64/arm64 免安装包及 SHA256SUMS
make installers    # 生成全部分发包：免安装包 + Windows x64/x86 NSIS 安装器 + Linux DEB/RPM

# 网页已构建时也可直接运行：
go run ./tools/package -installers -windows-x86
```

所有构建产物与 `SHA256SUMS` 清单均输出到被 Git 忽略的 `dist/` 目录，第三方依赖许可证会自动收集进包内。

## 打包配置说明

- `nfpm.yaml`：DEB / RPM 文件布局、依赖（`xdg-utils`，推荐 `zenity`）与元数据。
- `linux/foliochest.desktop` & `linux/refresh-menu.sh`：系统应用菜单入口与安装后桌面/图标缓存刷新。
- `windows/installer.nsi`：当前用户目录安装（`LocalAppData/Programs/FolioChest`，无需管理员权限）、开始菜单与可选桌面快捷方式；卸载按清单删除文件并仅清理指向本安装路径的 Run 启动项，保留用户资料与设备配置。
- `INSTALL.txt` / `README.txt`：随免安装包和系统安装包分发的简明使用与升级说明。
- `tools/package/`：编译多平台二进制、生成 Windows 图标资源、收集许可证并调用 nFPM/NSIS。
