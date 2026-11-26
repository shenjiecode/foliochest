// Query parameters select synthetic preview cases only; they are not authorization inputs.
export function previewOptions(search: string) {
  const params = new URLSearchParams(search);
  const unlocked = params.get("screen") !== "unlock";
  return {
    unlocked,
    pendingRequest: unlocked && params.get("approval") === "1",
  };
}

export type Policy = "allow" | "ask" | "deny";
export type ContentType = "Markdown" | "文本" | "JSON";
export type Attachment = { name: string; size: string };
export type Revision = {
  id: string;
  label: string;
  value: string;
  attachments: Attachment[];
};
export type Entry = {
  id: string;
  key: string;
  description: string;
  group: string;
  type: ContentType;
  value: string;
  read: Policy;
  write: Policy;
  deleted: boolean;
  attachments: Attachment[];
  history: Revision[];
};
export type View = "entries" | "approvals" | "trash" | "settings";
export type DialogKind =
  | "new"
  | "move"
  | "rename"
  | "delete"
  | "history"
  | "open-vault"
  | "create-vault"
  | "restore-vault"
  | "password"
  | null;
export const policyNames: Record<Policy, string> = {
  allow: "允许",
  ask: "需确认",
  deny: "拒绝",
};
const make = (
  id: string,
  key: string,
  description: string,
  group: string,
  type: ContentType,
  lines: string[],
): Entry => {
  const value = lines.join("\n");
  return {
    id,
    key,
    description,
    group,
    type,
    value,
    read: "ask",
    write: "ask",
    deleted: false,
    attachments: [],
    history: [
      { id: id + "-1", label: "初始版本 · 示例", value, attachments: [] },
    ],
  };
};
// Deliberately synthetic. These are not vault records or an implementation of the HTTP model.
export function exampleEntries(): Entry[] {
  const release = make(
    "release",
    "项目/发布清单",
    "版本交付前需要核对的事项",
    "工作资料",
    "Markdown",
    [
      "# 发布前检查清单",
      "",
      "记录每次交付前的核对项，确认后再发布。",
      "",
      "## 内容与附件",
      "",
      "- 核对本次交付的文件和版本。",
      "- 移除临时记录、测试账号与无关附件。",
      "- 检查说明中的链接与文件名。",
      "",
      "## 测试与回退",
      "",
      "1. 在独立环境中完成验证。",
      "2. 保留上一版本及对应备份。",
      "3. 确认恢复步骤可用。",
      "",
      "> 若仍有未解决的问题，先记录并处理，不跳过确认。",
      "",
      "## 交付记录",
      "",
      "| 项目 | 记录 |",
      "| --- | --- |",
      "| 版本 | 待填写 |",
      "| 验证环境 | 待填写 |",
      "| 备份位置 | 待填写 |",
    ],
  );
  release.attachments = [
    { name: "交付记录.txt", size: "示例附件" },
    { name: "manifest.json", size: "示例附件" },
  ];
  release.history[0].value = release.value.replace(
    /## 测试与回退[\s\S]*?## 交付记录/,
    "## 交付记录",
  );
  release.history.push({
    id: "release-2",
    label: "补充回退检查 · 示例",
    value: release.value,
    attachments: release.attachments.map((file) => ({ ...file })),
  });
  return [
    release,
    make(
      "meeting",
      "项目/会议记录",
      "讨论结论与后续事项",
      "工作资料",
      "Markdown",
      [
        "# 会议记录",
        "",
        "## 讨论结论",
        "",
        "- 保留必要的操作状态。",
        "- 将低频设置移出主工作区。",
        "",
        "## 后续事项",
        "",
        "完成页面预览后再确认实现范围。",
      ],
    ),
    make(
      "writing",
      "项目/文案约定",
      "界面用词和错误提示的约定",
      "工作资料",
      "Markdown",
      [
        "# 文案约定",
        "",
        "按钮写具体动作；错误说明原因和下一步。",
        "",
        "## 常用词",
        "",
        "- 新建条目",
        "- 保存修改",
        "- 移到回收站",
      ],
    ),
    make(
      "setup",
      "开发/环境说明",
      "本地开发环境与启动记录",
      "开发资料",
      "Markdown",
      [
        "# 环境说明",
        "",
        "这是一份合成的开发记录。",
        "",
        "## 启动前",
        "",
        "检查依赖，使用独立的测试目录。",
        "",
        "## 配置示例",
        "",
        "```text",
        "MODE=example",
        "HOST=127.0.0.1",
        "```",
      ],
    ),
    make(
      "api",
      "开发/接口配置",
      "供测试使用的接口配置样例",
      "开发资料",
      "JSON",
      [
        "{",
        '  "endpoint": "https://api.example.invalid",',
        '  "mode": "synthetic"',
        "}",
      ],
    ),
    make("reading", "个人/阅读摘记", "阅读中的想法与片段", "个人资料", "文本", [
      "把值得继续想的问题记下来。",
      "",
      "这份内容只用于展示纯文本资料的阅读效果。",
    ]),
    make("links", "个人/常用链接", "整理常用参考地址", "个人资料", "Markdown", [
      "# 常用链接",
      "",
      "- [示例站点](https://example.org)",
      "",
      "所有链接均为示例。",
    ]),
    make("inbox", "待整理的想法", "还没有归组的一则记录", "", "文本", [
      "给没有归属的资料留一个入口。",
    ]),
    {
      ...make(
        "archived",
        "项目/旧版检查清单",
        "已经被新版替代的示例条目",
        "工作资料",
        "Markdown",
        ["# 旧版检查清单", "", "- 核对文件。", "- 保留备份。"],
      ),
      value: "",
      deleted: true,
    },
  ];
}
export function groupsOf(entries: Entry[]) {
  return [...new Set(entries.map((entry) => entry.group).filter(Boolean))];
}
export function visibleEntries(
  entries: Entry[],
  group: string | null,
  query: string,
  body: boolean,
  trash: boolean,
) {
  const q = query.trim().toLocaleLowerCase();
  return entries.filter(
    (entry) =>
      entry.deleted === trash &&
      (group === null || entry.group === group) &&
      [
        entry.key,
        entry.description,
        entry.group,
        ...(body && !trash ? [entry.value] : []),
      ]
        .join("\n")
        .toLocaleLowerCase()
        .includes(q),
  );
}
export function saveExample(entries: Entry[], next: Entry): Entry[] {
  return entries.map((entry) =>
    entry.id !== next.id
      ? entry
      : {
          ...next,
          history: [
            ...entry.history,
            {
              id: entry.id + "-" + (entry.history.length + 1),
              label: "本次预览修改",
              value: next.value,
              attachments: next.attachments.map((file) => ({ ...file })),
            },
          ],
        },
  );
}
