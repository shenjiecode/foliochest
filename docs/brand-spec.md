# FolioChest / 资料匣 · 页面改版参考

- 品牌标识沿用现有 Georgia 文字字标 `FolioChest`，中文名称为“资料匣”。项目未提供独立图形 Logo；本轮不新造、不冒用外部 Logo。
- 字标原实现：`web/src/main.ts` 与 `web/src/style.css` 的 `.wordmark`；预览复用实现：`web/src/preview/shared.tsx` 的 `Brand`。
- 以原界面源码和用户提供的截图为输入；旧截图属于本地参考，不纳入仓库。新预览截图生成在被忽略的 `.local/ui-v0-*.png`。
- 配色、字体、间距、组件、滚动和响应式规则统一见 [DESIGN §15](DESIGN.md#15-网页整体改版--可点击设计预览) 及文件头的机器可读 tokens。
- 不使用产品配图、外部字体或 CDN。操作图标由固定版本的 `lucide-react` 本地打包，只表达动作，不充当品牌标识。
- 页面数据来自 `web/src/preview/model.ts` 的合成示例。真实资料、主密码、附件或凭据不作为设计素材。
