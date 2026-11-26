# 分发构建

## 工具

- 项目自身：README 指定的 Go、Node.js；不为打包改变应用的 Go module 或运行依赖。
- Linux 安装包：[nFPM](https://nfpm.goreleaser.com/)，本轮使用 **2.49.0**。
- Windows 安装程序：[NSIS](https://nsis.sourceforge.io/)，本轮使用 **3.11 Unicode**。脚本使用 zlib 压缩；上游声明保存在 `windows/NSIS-LICENSE.txt` 并随安装程序分发。
- 工具只在构建机需要，最终用户不需要安装上述工具。

nFPM 可使用官方预编译程序，也可执行：

```sh
go install github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.49.0
```

该工具版本需要 Go 1.27.4 或更新，Go 可能自动下载工具链；不会修改项目的 Go 1.26 配置。NSIS 可由构建机的系统包管理器安装，例如 Debian/Ubuntu 的 `nsis` 或 Fedora 的 `mingw32-nsis`。

本轮在 Linux 构建机上生成全部目标；没有在 Windows 上执行安装器。工具安装位置可通过 `NFPM`、`MAKENSIS` 环境变量指定，默认从 PATH 查找。使用独立解压的 NSIS 时，可用 `NSISDIR` 指向其资源目录。

## 命令

```sh
make package       # 默认三个免安装压缩包
make installers    # 免安装包 + Windows x64/x86 安装器 + Linux DEB/RPM

# 网页已构建时，直接调用：
go run ./tools/package -installers
go run ./tools/package -installers -windows-x86
```

`-windows-x86` 增加实验性的 Windows 32 位目标；默认 Windows 目标仍为 amd64。包文件名中的 `amd64` 就是 x64，`386` 就是 x86（32 位）。Linux `amd64/arm64` 在 RPM 元信息中对应 `x86_64/aarch64`。

所有产物与 SHA-256 清单位于被 Git 忽略的 `dist/`。Windows 资源文件只临时生成，第三方许可随包收集；不扫描资料库或本机设备配置。工具不会执行应用、安装生成的包、发布 Releases 或修改仓库可见性。

## 配置职责

- `nfpm.yaml`：DEB/RPM 文件、依赖和元信息。
- `linux/foliochest.desktop`：系统应用菜单入口。
- `linux/refresh-menu.sh`：只刷新应用/图标缓存，不启动进程或接触家目录。
- `windows/installer.nsi`：当前用户安装、开始菜单入口、可选桌面快捷方式、卸载登记。
- `INSTALL.txt`：用户安装、升级与卸载说明。
- `tools/package/`：编译和组织产物，调用 nFPM/NSIS，不自行编码安装包格式。

Windows 固定默认安装目录为当前用户的 `LocalAppData/Programs/FolioChest`，不请求管理员权限。x64/x86 共用产品登记，切换架构需先卸载旧版。安装/卸载检测程序文件是否可写，不强杀进程；卸载只按文件清单删除，不递归清空整个安装目录，并清理指向本安装路径的资料匣 Run 项。

Linux 使用包管理器管理 `/usr/bin` 和 `/usr/share` 下的程序文件；升级/卸载前由使用者退出程序并关闭登录自启动。不自动清理各用户的设备凭据或家目录。

## GitHub Actions

`build.yml` 在 main/develop 提交时执行构建，`release.yml` 在版本 tag 推送时调用本工具生成安装包并发布 Release。tag 版本通过 `-version` 同时注入应用和包元信息，详细规则见 [发布流程](../docs/RELEASING.md)。workflow 不运行应用或安装器，也不代表真机验收。

## 验证边界

已做编译、安装包构建、架构/依赖/文件清单及校验和检查。未执行安装脚本，也未在 Windows/Fedora/Debian 真机完成安装、升级、卸载回归。x86 仅为实验性构建，不等于已验收支持。当前没有代码签名、自动更新或 macOS 安装包。
