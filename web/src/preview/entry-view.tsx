import { useState } from "react";
import {
  ArrowLeft,
  Bot,
  Download,
  History,
  MoreHorizontal,
  Paperclip,
  Pencil,
  Plus,
  Trash2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { renderMarkdown } from "../markdown";
import { EmptyState, Field, IconButton, SelectField } from "./shared";
import { policyNames, type DialogKind, type Entry, type Policy } from "./model";

export function EntryView({
  entry,
  groups,
  onSave,
  onDirty,
  onOpen,
  onBack,
}: {
  entry: Entry;
  groups: string[];
  onSave: (entry: Entry) => void;
  onDirty: (dirty: boolean) => void;
  onOpen: (kind: DialogKind) => void;
  onBack: () => void;
}) {
  const [tab, setTab] = useState("body");
  const [editing, setEditing] = useState(!entry.value && !entry.deleted);
  const [draft, setDraft] = useState(entry);
  const [changed, setChanged] = useState(false);
  const active = editing ? draft : entry;
  const update = (patch: Partial<Entry>) => {
    const next = { ...draft, ...patch };
    const dirty = ["value", "description", "group", "read", "write"].some(
      (key) => next[key as keyof Entry] !== entry[key as keyof Entry],
    );
    setDraft(next);
    setChanged(dirty);
    onDirty(dirty);
  };
  const save = () => {
    onSave(draft);
    setChanged(false);
    setEditing(false);
  };
  const cancel = () => {
    setDraft(entry);
    setChanged(false);
    setEditing(false);
    onDirty(false);
  };
  return (
    <section
      className="entry-panel"
      aria-label={"条目：" + entry.key}
      onKeyDown={(event) => {
        if (editing && (event.ctrlKey || event.metaKey) && event.key === "s") {
          event.preventDefault();
          if (changed) save();
        }
      }}
    >
      <header className="entry-heading">
        <IconButton
          className="back-to-list"
          label="返回资料列表"
          onClick={onBack}
        >
          <ArrowLeft />
        </IconButton>
        <div className="entry-heading-copy">
          <h2>{entry.key}</h2>
          {entry.description && <p>{entry.description}</p>}
        </div>
        <div className="heading-actions">
          <IconButton label="查看历史版本" onClick={() => onOpen("history")}>
            <History />
          </IconButton>
          {!entry.deleted && (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="icon" aria-label="条目更多操作">
                  <MoreHorizontal />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onSelect={() => onOpen("rename")}>
                  <Pencil />
                  重命名 Key
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  variant="destructive"
                  onSelect={() => onOpen("delete")}
                >
                  <Trash2 />
                  移到回收站
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          )}
        </div>
      </header>
      {entry.deleted ? (
        <div className="content-pane">
          <EmptyState
            icon={<Trash2 />}
            title="这条资料已移到回收站"
            action={
              <Button onClick={() => onOpen("history")}>
                <History />
                查看可恢复版本
              </Button>
            }
          >
            内容保留在删除前的历史版本中。
          </EmptyState>
        </div>
      ) : (
        <Tabs value={tab} onValueChange={setTab} className="entry-tabs">
          <div className="editor-toolbar">
            <TabsList aria-label="条目内容">
              <TabsTrigger value="body">正文</TabsTrigger>
              <TabsTrigger value="attachments">
                附件
                <span className="tab-count">{entry.attachments.length}</span>
              </TabsTrigger>
              <TabsTrigger value="properties">属性</TabsTrigger>
            </TabsList>
            <div className="editor-actions">
              {editing ? (
                <>
                  {changed && <span className="draft-label">未保存</span>}
                  <Button variant="ghost" size="sm" onClick={cancel}>
                    取消
                  </Button>
                  <Button size="sm" disabled={!changed} onClick={save}>
                    保存
                  </Button>
                </>
              ) : (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    setDraft(entry);
                    setEditing(true);
                  }}
                >
                  <Pencil />
                  编辑
                </Button>
              )}
            </div>
          </div>
          <TabsContent value="body" className="content-pane">
            {editing ? (
              <textarea
                className="source-editor"
                aria-label="条目正文"
                spellCheck={false}
                value={draft.value}
                onChange={(event) => update({ value: event.target.value })}
                placeholder="写下这条资料的内容…"
              />
            ) : entry.type === "Markdown" ? (
              <article
                className="article"
                dangerouslySetInnerHTML={{
                  __html: renderMarkdown(entry.value),
                }}
              />
            ) : (
              <article
                className={
                  "article plain-content " +
                  (entry.type === "JSON" ? "code-content" : "")
                }
              >
                {entry.value}
              </article>
            )}
          </TabsContent>
          <TabsContent value="attachments" className="content-pane">
            <div className="attachments-panel">
              <div className="attachment-heading">
                <h3>附件</h3>
                <Button variant="outline" size="sm" disabled>
                  <Plus />
                  添加附件
                </Button>
              </div>
              {entry.attachments.length ? (
                entry.attachments.map((file) => (
                  <div className="attachment-row" key={file.name}>
                    <Paperclip />
                    <div>
                      <p>{file.name}</p>
                      <small>{file.size}</small>
                    </div>
                    <IconButton label="下载附件（预览未接入）" disabled>
                      <Download />
                    </IconButton>
                  </div>
                ))
              ) : (
                <EmptyState icon={<Paperclip />} title="暂无附件" />
              )}
              <p className="settings-footnote">
                仅展示附件样式，不读取真实文件。
              </p>
            </div>
          </TabsContent>
          <TabsContent value="properties" className="content-pane">
            <div className="property-panel">
              <section className="property-section">
                <h3 className="section-heading">公开信息</h3>
                <div className="field-grid">
                  <Field id="entry-key" label="Key">
                    <Input
                      id="entry-key"
                      value={entry.key}
                      readOnly
                      className="font-mono"
                    />
                  </Field>
                  <Field id="entry-description" label="描述">
                    {editing ? (
                      <Textarea
                        id="entry-description"
                        value={draft.description}
                        onChange={(event) =>
                          update({ description: event.target.value })
                        }
                      />
                    ) : (
                      <p id="entry-description" className="static-field">
                        {entry.description || "无"}
                      </p>
                    )}
                  </Field>
                  <Field id="entry-group" label="分组">
                    <SelectField
                      id="entry-group"
                      value={active.group}
                      disabled={!editing}
                      onChange={(event) =>
                        update({ group: event.target.value })
                      }
                    >
                      <option value="">未分组</option>
                      {groups.map((name) => (
                        <option key={name} value={name}>
                          {name}
                        </option>
                      ))}
                    </SelectField>
                  </Field>
                </div>
              </section>
              <section className="property-section">
                <h3 className="section-heading">AI 权限</h3>
                <div className="field-pair">
                  {(["read", "write"] as const).map((key) => (
                    <Field
                      key={key}
                      id={"policy-" + key}
                      label={key === "read" ? "读取" : "修改"}
                    >
                      <SelectField
                        id={"policy-" + key}
                        value={active[key]}
                        disabled={!editing}
                        onChange={(event) =>
                          update({ [key]: event.target.value as Policy })
                        }
                      >
                        {Object.entries(policyNames).map(([value, label]) => (
                          <option value={value} key={value}>
                            {label}
                          </option>
                        ))}
                      </SelectField>
                    </Field>
                  ))}
                </div>
              </section>
            </div>
          </TabsContent>
        </Tabs>
      )}
      <footer className="entry-status">
        <span>
          {entry.type} · {active.value.length} 字符
          {entry.deleted ? " · 已删除" : ""}
        </span>
        {!entry.deleted && (
          <button
            className="policy-summary"
            onClick={() => setTab("properties")}
            aria-label="查看 AI 读取与修改权限"
          >
            <Bot />
            <span>
              读取{policyNames[active.read]} · 修改{policyNames[active.write]}
            </span>
          </button>
        )}
      </footer>
    </section>
  );
}
