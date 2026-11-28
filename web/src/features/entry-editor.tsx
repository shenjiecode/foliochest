import { useEffect, useRef, useState, type DragEvent } from "react";
import {
  ArrowLeft,
  History,
  MoreHorizontal,
  Pencil,
  ShieldCheck,
  FileText,
  UploadCloud,
} from "lucide-react";
import { Button } from "@/components/ui/button";
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
import { BodyEditor } from "@/components/body-editor";
import { ChoiceField, formatChoices } from "@/components/choice-field";
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
  onCancel,
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
  onCancel: () => void;
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
  const [dragging, setDragging] = useState(false);
  const dragDepth = useRef(0);
  const [editing, setEditing] = useState(isNew),
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
  const canAdd = !!draft && editing && !unavailable && !loading && !task.busy;
  const addFiles = (files: File[]) => {
    if (!canAdd || !draft || !files.length) return;
    void task.run(
      (ctx) => addAttachments(draft.attachments || [], files, ctx.assert),
      (attachments) => update({ attachments }),
    );
  };
  const isFileDrag = (event: DragEvent) =>
    Array.from(event.dataTransfer.types).includes("Files");
  return (
    <form
      className="entry-panel"
      aria-label={isNew ? "新建资料" : entryKey}
      onDragEnterCapture={(e) => {
        if (!isFileDrag(e)) return;
        e.preventDefault();
        dragDepth.current++;
        setDragging(true);
      }}
      onDragOverCapture={(e) => {
        if (!isFileDrag(e)) return;
        e.preventDefault();
        e.stopPropagation();
        e.dataTransfer.dropEffect = canAdd ? "copy" : "none";
      }}
      onDragLeaveCapture={(e) => {
        if (!isFileDrag(e)) return;
        dragDepth.current = Math.max(0, dragDepth.current - 1);
        if (!dragDepth.current) setDragging(false);
      }}
      onDropCapture={(e) => {
        if (!isFileDrag(e) && !e.dataTransfer.files.length) return;
        e.preventDefault();
        e.stopPropagation();
        dragDepth.current = 0;
        setDragging(false);
        if (!canAdd) {
          client.notice("请先进入编辑状态，并等待当前操作完成。");
          return;
        }
        if (
          Array.from(e.dataTransfer.items).some(
            (item) => item.webkitGetAsEntry?.()?.isDirectory,
          )
        ) {
          client.notice("请拖入文件，不支持拖入文件夹。");
          return;
        }
        addFiles(Array.from(e.dataTransfer.files));
      }}
      onSubmit={(e) => {
        e.preventDefault();
        if (editing && !loading && !task.busy) void save();
      }}
      onKeyDown={(e) => {
        if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "s") {
          e.preventDefault();
          if (editing && !loading && !task.busy) void save();
        }
      }}
    >
      {dragging && (
        <div className="attachment-drag-overlay" role="status">
          <UploadCloud />
          <strong>
            {canAdd ? "松开以添加附件" : "请先进入编辑状态或等待当前操作完成"}
          </strong>
          <span>文件将加入当前草稿，保存后生效</span>
        </div>
      )}
      <header className="entry-heading">
        <IconButton
          className="back-to-list"
          label="返回列表"
          type="button"
          onClick={onBack}
        >
          <ArrowLeft />
        </IconButton>
        <h1 className="entry-title">{isNew ? "新建资料" : entryKey}</h1>
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
        {draft && !unavailable && (
          <div className="editor-actions">
            {editing ? (
              <>
                <Button
                  type="button"
                  variant="outline"
                  disabled={task.busy || loading}
                  onClick={onCancel}
                >
                  取消
                </Button>
                <Button type="submit" disabled={task.busy || loading}>
                  {task.busy ? "处理中…" : "保存"}
                </Button>
              </>
            ) : (
              <Button
                type="button"
                variant="outline"
                disabled={task.busy || loading}
                onClick={() => setEditing(true)}
              >
                <Pencil />
                编辑
              </Button>
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
          资料已被更新。
          {dirty.current ? "未保存的草稿已保留。" : "正在重新加载最新版本…"}
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
          <div className="entry-document">
            <Properties
              entry={draft}
              groups={groups}
              format={format}
              editable={editing}
              isNew={isNew}
              onChange={update}
            />
            <section className="entry-body" aria-label="正文">
              <div className="body-format">
                <ChoiceField
                  id="entry-format"
                  label="正文格式"
                  value={draft.format}
                  options={formatChoices}
                  disabled={!editing || task.busy || loading}
                  onChange={(format) => update({ format })}
                />
              </div>
              {editing ? (
                <BodyEditor
                  value={draft.value}
                  format={draft.format}
                  disabled={task.busy || loading}
                  onChange={(value) => update({ value })}
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
            </section>
            <Attachments
              files={draft.attachments || []}
              editable={editing}
              busy={task.busy || loading}
              onAdd={addFiles}
              onRemove={(i) =>
                update({
                  attachments: (draft.attachments || []).filter(
                    (_, n) => n !== i,
                  ),
                })
              }
            />
          </div>
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
