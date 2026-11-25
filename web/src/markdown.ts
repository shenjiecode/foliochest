import MarkdownIt from "markdown-it";
import { esc } from "./ui";
const markdown = new MarkdownIt({
  html: false,
  linkify: false,
  typographer: false,
});
// Do not fetch any images, including tracking URLs and data URLs.
markdown.renderer.rules.image = (tokens, index) =>
  `<span class="muted">[图片：${esc(tokens[index].content)}]</span>`;
markdown.validateLink = (url) => /^(https?:|mailto:)/i.test(url);
const linkOpen = markdown.renderer.rules.link_open;
markdown.renderer.rules.link_open = (tokens, index, options, env, self) => {
  tokens[index].attrSet("target", "_blank");
  tokens[index].attrSet("rel", "noopener noreferrer");
  return linkOpen
    ? linkOpen(tokens, index, options, env, self)
    : self.renderToken(tokens, index, options);
};
export function renderMarkdown(source: string): string {
  return markdown.render(source);
}
