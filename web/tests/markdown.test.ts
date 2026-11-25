import assert from "node:assert/strict";
import { renderMarkdown } from "../src/markdown";
const text = renderMarkdown(
  "# Heading\n\n**bold**\n\n- one\n- two\n\n```js\nalert(1)\n```",
);
assert.match(text, /<h1>Heading<\/h1>/);
assert.match(text, /<strong>bold<\/strong>/);
assert.match(text, /<ul>/);
assert.match(text, /<code class="language-js">/);
for (const source of [
  "<script>alert(1)</script>",
  "<img src=x onerror=alert(1)>",
  "<svg/onload=alert(1)>",
  '<iframe src="https://example.com"></iframe>',
]) {
  const out = renderMarkdown(source);
  assert.doesNotMatch(out, /<(?:script|img|svg|iframe)\b/i);
}
for (const url of [
  "javascript:alert(1)",
  "vbscript:exec(1)",
  "data:text/html,hello",
  "file:///etc/passwd",
  "/api/backup",
  "//tracker.example/x",
]) {
  assert.doesNotMatch(renderMarkdown("[link](" + url + ")"), /<a\s/i);
}
const image = renderMarkdown("![<img src=x>](https://tracker.example/pixel)");
assert.doesNotMatch(image, /<img\b|<[^>]+\ssrc\s*=|https:\/\/tracker/);
assert.match(image, /&lt;img/);
const link = renderMarkdown("[safe](https://example.com)");
assert.match(link, /target="_blank"/);
assert.match(link, /rel="noopener noreferrer"/);
const code = renderMarkdown('```"><img src=x>\n<svg/onload=alert(1)>\n```');
assert.doesNotMatch(code, /<img\b|<svg\b/);
console.log("Markdown rendering and unsafe-content tests passed.");
