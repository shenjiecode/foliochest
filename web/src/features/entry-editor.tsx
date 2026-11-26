import { useEffect, useRef, useState } from "react";
import {
  ArrowLeft,
  History,
  MoreHorizontal,
  Pencil,
  ShieldCheck,
  FileText,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { EmptyState, IconButton } from "@/components/common";
import { ErrorNotice, Notice } from "@/components/forms";
import { useClient, useTask } from "@/lib/hooks";
import { errorText, Sequence, StaleRequest } from "@/lib/client";
import { addAttachments } from "@/lib/files";
import {
  type Entry,
  type Summary,
  type Group,
  policyLabels,
  stateLabels,
} from "@/lib/types";
import { renderMarkdown } from "@/markdown";
import { Attachments, Properties } from "./document-fields";

export function EntryEditor({
  entryKey,
  summary,
  groups,
  format,
  initialGroup,
  onDirty,
  onBusy,
  onSaved,
  onManage,
  onHistory,
  onBack,
}: {
  entryKey: string | null;
  summary?: Summary;
  groups: Group[];
  format: number;
  initialGroup: string;
  onDirty: (dirty: boolean) => void;
  onBusy: (busy: boolean) => void;
  onSaved: (key: string) => void;
  onManage: (kind: "rename" | "delete", entry: Entry) => void;
  onHistory: () => void;
  onBack: () => void;
}) {
  const isNew = entryKey === null,
    client = useClient(),
    task = useTask(),
    sequence = useRef(new Sequence()),
    dirty = useRef(false);
  const [draft, setDraft] = useState<Entry | null>(() =>
    isNew
      ? {
          entry_id: "",
          key: "",
          description: "",
          group: initialGroup,
          revision: "",
          read_policy: "ask",
          write_policy: "ask",
          value: "",
          format: "markdown",
          attachments: [],
          updated_at: "",
        }
      : null,
  );
  const [editing, setEditing] = useState(isNew),
    [tab, setTab] = useState(isNew ? "properties" : "body"),
    [loading, setLoading] = useState(!isNew),
    [loadError, setLoadError] = useState(""),
    [retry, setRetry] = useState(0);
  const update = (patch: Partial<Entry>) => {
    dirty.current = true;
    onDirty(true);
    setDraft((before) => (before ? { ...before, ...patch } : before));
  };
  useEffect(() => {
    onBusy(task.busy);
    return () => onBusy(false);
  }, [task.busy, onBusy]);
  useEffect(() => {
    if (isNew || dirty.current || task.busy) return;
    if (summary && summary.state !== "ready") {
      setLoading(false);
      return;
    }
    const ticket = sequence.current.next();
    let active = true;
    setLoading(true);
    setLoadError("");
    client
      .request<Entry>("/api/entry?key=" + encodeURIComponent(entryKey))
      .then((entry) => {
        if (active && sequence.current.current(ticket) && !dirty.current) {
          setDraft(entry);
        }
      })
      .catch((e) => {
        if (active && !(e instanceof StaleRequest)) setLoadError(errorText(e));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
      sequence.current.next();
    };
  }, [
    client,
    entryKey,
    summary?.revision,
    summary?.state,
    isNew,
    task.busy,
    retry,
  ]);
  const save = () =>
    task.run(
      async (ctx) => {
        if (!draft) throw new Error("请先读取资料");
        if (!draft.key.trim() || !draft.description.trim())
          throw new Error("请在属性中填写 Key 和描述");
        if (new TextEncoder().encode(draft.value).length > 1024 * 1024)
          throw new Error("正文最大为 1 MiB");
        await ctx.request("/api/entries/save", {
          changes: [
            {
              key: draft.key,
              expected_revision: draft.revision,
              description: draft.description,
              ...(format >= 3 ? { group: draft.group || "" } : {}),
              value: draft.value,
              format: draft.format,
              attachments: draft.attachments || [],
              read_policy: draft.read_policy,
              write_policy: draft.write_policy,
            },
          ],
        });
        return await ctx.request<Entry>(
          "/api/entry?key=" + encodeURIComponent(draft.key),
        );
      },
      (saved) => {
        dirty.current = false;
        onDirty(false);
        setDraft(saved);
        setEditing(false);
        onSaved(saved.key);
        client.notice("资料已保存。");
      },
    );
  const stale =
    !!draft &&
    !isNew &&
    !!summary &&
    (summary.state !== "ready" || summary.revision !== draft.revision);
  const unavailable = summary && summary.state !== "ready" && !dirty.current;
  return (
    <form
      className="entry-panel"
      onSubmit={(e) => {
        e.preventDefault();
        void save();
      }}
      onKeyDown={(e) => {
        if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "s") {
          e.preventDefault();
          if (editing && !loading && !task.busy) void save();
        }
      }}
    >
      <header className="entry-heading">
        <IconButton
          className="back-to-list"
          label="返回列表"
          type="button"
          onClick={onBack}
        >
          <ArrowLeft />
        </IconButton>
        <div className="entry-heading-copy">
          <h2>{isNew ? "新建资料" : entryKey}</h2>
          <p>{draft?.description || summary?.description}</p>
        </div>
        {!isNew && (
          <div className="heading-actions">
            <IconButton
              label="历史与恢复"
              type="button"
              onClick={onHistory}
              disabled={task.busy}
            >
              <History />
            </IconButton>
            {draft && !unavailable && (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    type="button"
                    aria-label="资料操作"
                    disabled={task.busy}
                  >
                    <MoreHorizontal />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent>
                  <DropdownMenuItem
                    disabled={format < 2}
                    onSelect={() => onManage("rename", draft)}
                  >
                    重命名 Key
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    disabled={format < 2}
                    onSelect={() => onManage("delete", draft)}
                  >
                    移到回收站
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            )}
          </div>
        )}
      </header>
      <ErrorNotice message={loadError || task.error} />
      {loadError && (
        <Button
          type="button"
          variant="ghost"
          onClick={() => setRetry((v) => v + 1)}
        >
          重试读取
        </Button>
      )}
      {stale && (
        <Notice>
          当前版本已变化。
          {dirty.current
            ? "未保存的草稿已保留，保存仍会校验原版本。"
            : "正在重新读取；下方内容是先前版本。"}
        </Notice>
      )}
      {unavailable ? (
        <EmptyState
          icon={<History />}
          title={stateLabels[summary.state] || summary.state}
          action={
            <Button type="button" onClick={onHistory}>
              查看历史与恢复
            </Button>
          }
        />
      ) : !draft ? (
        <EmptyState
          icon={<FileText />}
          title={loading ? "正在读取资料…" : "暂时无法读取"}
        />
      ) : (
        <fieldset
          className="entry-fields"
          disabled={task.busy || loading}
          aria-busy={task.busy || loading}
        >
          <Tabs className="entry-tabs" value={tab} onValueChange={setTab}>
            <div className="editor-toolbar">
              <TabsList>
                <TabsTrigger value="body">正文</TabsTrigger>
                <TabsTrigger value="attachments">
                  附件
                  <span className="tab-count">
                    {draft.attachments?.length || 0}
                  </span>
                </TabsTrigger>
                <TabsTrigger value="properties">属性</TabsTrigger>
              </TabsList>
              <div className="editor-actions">
                {editing ? (
                  <>
                    <span className="draft-label">
                      {dirty.current ? "未保存" : "编辑中"}
                    </span>
                    <Button type="submit" disabled={task.busy || loading}>
                      {task.busy ? "处理中…" : "保存"}
                    </Button>
                  </>
                ) : (
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => setEditing(true)}
                  >
                    <Pencil />
                    编辑
                  </Button>
                )}
              </div>
            </div>
            <TabsContent className="content-pane" value="body">
              {editing ? (
                <textarea
                  className="source-editor"
                  aria-label="正文"
                  value={draft.value}
                  onChange={(e) => update({ value: e.target.value })}
                  spellCheck={false}
                />
              ) : draft.format === "markdown" ? (
                <article
                  className="article"
                  dangerouslySetInnerHTML={{
                    __html: renderMarkdown(draft.value),
                  }}
                />
              ) : (
                <pre className="article plain-content code-content">
                  {draft.value}
                </pre>
              )}
            </TabsContent>
            <TabsContent className="content-pane" value="attachments">
              <Attachments
                files={draft.attachments || []}
                editable={editing}
                busy={task.busy}
                onAdd={(files) =>
                  void task.run(
                    (ctx) =>
                      addAttachments(
                        draft.attachments || [],
                        files,
                        ctx.assert,
                      ),
                    (attachments) => update({ attachments }),
                  )
                }
                onRemove={(i) =>
                  update({
                    attachments: (draft.attachments || []).filter(
                      (_, n) => n !== i,
                    ),
                  })
                }
              />
            </TabsContent>
            <TabsContent className="content-pane" value="properties">
              <Properties
                entry={draft}
                groups={groups}
                format={format}
                editable={editing}
                isNew={isNew}
                onChange={update}
              />
            </TabsContent>
          </Tabs>
        </fieldset>
      )}
      {draft && (
        <footer className="entry-status">
          <span>
            {isNew ? "尚未保存" : `版本 ${draft.revision.slice(0, 8)}`}
          </span>
          <span className="policy-summary">
            <ShieldCheck />
            AI 读取：{policyLabels[draft.read_policy]} · 修改：
            {policyLabels[draft.write_policy]}
          </span>
        </footer>
      )}
    </form>
  );
}
