资料匣 · FolioChest — 免安装版

Windows
  完整解压到固定目录，双击 foliochest.exe 自动打开默认浏览器（无控制台窗口）。
  当前未做代码签名，运行前请核对来源与 SHA-256。

Linux
  解压到固定目录后执行 ./foliochest。
  添加当前用户的应用菜单入口：sh install-shortcut.sh
  移除应用入口：sh install-shortcut.sh --remove
  系统目录选择器需要 zenity 等辅助工具。

日常使用
  默认地址：http://127.0.0.1:7463/
  仅启动后台不打开浏览器：foliochest --background
  关闭网页不会停止后台服务；退出请使用网页中的“退出资料匣”。
  本机配置默认位于系统配置目录的 keyfolio/ 子目录，请勿放入资料库同步目录。

升级与许可
  升级前请先保存草稿并从网页退出旧程序，再替换程序文件。
  本项目采用 MIT License（见 LICENSE）；第三方许可见 THIRD_PARTY_LICENSES 与 SHADCN_NOTICE.md。
