# FolioChest / 资料匣 · 界面品牌

- 品牌标识以中文“资料匣”在上、英文 `FolioChest` 在下排列；中文使用本机字体，英文保留 Georgia。网页标题为“资料匣 · FolioChest”。没有独立图形 Logo，不新造或冒用外部 Logo。
- 字标实现：`web/src/components/common.tsx` 的 `Brand`；样式位于 `web/src/styles/workspace.css`。
- 配色、字体、间距、组件及滚动规则见 [DESIGN §14](DESIGN.md#14-网页界面) 和文件头 tokens；当前为左侧合并资料树与右侧编辑区的两栏工具界面。
- 不使用产品配图、外部字体或 CDN；固定版本的 `lucide-react` 本地打包，只表达操作，不充当品牌标识。
- 页面内容来自真实资料库 API。测试只使用合成资料，截图和测试运行目录不提交；真实资料、主密码和凭据不作为设计素材。
