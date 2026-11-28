# 品牌 Logo 与应用图标

采用用户指定的透明 PNG，原图保存在 `logo-original.png`。不重绘图形，不改变配色；生成时裁去透明留白、等比例缩放并居中留边，保留透明通道。

- `logo.png`：512×512，README 与网页品牌图形。
- `foliochest.png`：256×256，Linux 应用菜单/桌面快捷方式及分发包图标。
- `foliochest.ico`：16/24/32/48/64/128/256 多尺寸，Windows EXE/安装器/快捷方式图标，同时作为网页 favicon。
- `favicon-32.png`：32×32，网页 PNG favicon。
- `apple-touch-icon.png`：180×180，浏览器收藏/主屏幕图标。

需重新生成时，安装 Pillow 后运行 `python3 packaging/assets/generate-icons.py`。正常网页构建与打包使用已生成资源，不依赖 Python/Pillow。

`web/scripts/build.mjs` 将网页资源复制进 `web/dist/`，随 Go 程序嵌入；桌面打包沿用同一组 PNG/ICO。
