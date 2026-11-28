import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Field } from "@/components/common";
import { BodyEditor } from "@/components/body-editor";
import { ChoiceField, formatChoices } from "@/components/choice-field";
import {
  ErrorNotice,
  GroupField,
  Modal,
  Notice,
  useConfirm,
} from "@/components/forms";
import { useClient, useDirty, useResource, useTask } from "@/lib/hooks";
import {
  type Entry,
  type Group,
  type History as HistoryData,
} from "@/lib/types";
import { groupLabel } from "@/lib/groups";
import { Attachments } from "./document-fields";

export function historyRules(history: HistoryData) {
  const heads = history.versions.filter((v) => v.head);
  const groups = [...new Set(heads.map((v) => v.group || ""))];
  const collision = new Set(heads.map((v) => v.entry_id)).size > 1;
  return {
    groups,
    groupConflict: groups.length > 1,
    blocked: collision
      ? "存在同名冲突，暂不支持直接合并或删除。"
      : !["ready", "conflict", "deleted"].includes(history.state)
        ? "同步尚未完整，当前仅可查看历史版本。"
        : "",
  };
}
export function HistoryDialog({
  entryKey,
  format,
  groups,
  onClose,
  onChanged,
}: {
  entryKey: string;
  format: number;
  groups: Group[];
  onClose: () => void;
  onChanged: (key: string, deleted?: boolean) => void;
}) {
  const [revision, setRevision] = useState(0),
    [dirty, setDirty] = useState(false),
    [busy, setBusy] = useState(false),
    confirm = useConfirm();
  const history = useResource<HistoryData>(
    "/api/history?key=" + encodeURIComponent(entryKey),
    revision,
  );
  useDirty(dirty);
  const discard = () =>
    !dirty
      ? Promise.resolve(true)
      : confirm({
          title: "放弃历史编辑区的修改？",
          description: "未保存的合并草稿将丢失。",
          label: "放弃修改",
          danger: true,
        });
  return (
    <Modal
      title="历史与恢复"
      description={entryKey}
      busy={busy}
      wide
      onClose={() =>
        void discard().then((ok) => {
          if (ok) onClose();
        })
      }
      footer={
        <>
          <Button
            variant="outline"
            disabled={busy}
            onClick={() =>
              void discard().then((ok) => {
                if (ok) {
                  setDirty(false);
                  setRevision((n) => n + 1);
                }
              })
            }
          >
            重新读取版本
          </Button>
          <Button
            variant="outline"
            disabled={busy}
            onClick={() =>
              void discard().then((ok) => {
                if (ok) onClose();
              })
            }
          >
            关闭
          </Button>
        </>
      }
    >
      <ErrorNotice message={history.error} />
      {history.loading && <p role="status">正在读取历史…</p>}
      {history.data && (
        <HistoryContent
          key={revision}
          history={history.data}
          format={format}
          groups={groups}
          busy={busy}
          dirty={dirty}
          onDirty={setDirty}
          onBusy={setBusy}
          discard={discard}
          onChanged={(key, deleted) => {
            setDirty(false);
            onChanged(key, deleted);
            onClose();
          }}
        />
      )}
    </Modal>
  );
}
function HistoryContent({
  history,
  format,
  groups,
  busy,
  dirty,
  onDirty,
  onBusy,
  discard,
  onChanged,
}: {
  history: HistoryData;
  format: number;
  groups: Group[];
  busy: boolean;
  dirty: boolean;
  onDirty: (value: boolean) => void;
  onBusy: (value: boolean) => void;
  discard: () => Promise<boolean>;
  onChanged: (key: string, deleted?: boolean) => void;
}) {
  const [selected, setSelected] = useState(
    history.versions.find((v) => v.available && !v.deleted)?.revision ||
      history.versions.find((v) => v.available)?.revision ||
      "",
  );
  const detail = useResource<Entry>(
    selected
      ? "/api/history/entry?key=" +
          encodeURIComponent(history.key) +
          "&revision=" +
          encodeURIComponent(selected)
      : null,
  );
  const rules = historyRules(history),
    task = useTask(),
    confirm = useConfirm(),
    client = useClient();
  const remove = () =>
    task.run(
      async (ctx) => {
        if (
          !(await confirm({
            title: "删除全部当前分支？",
            description:
              "全部当前分支将移到回收站，未保存的合并草稿会丢失。",
            label: "删除全部分支",
            danger: true,
          }))
        )
          return false;
        onBusy(true);
        try {
          await ctx.request("/api/entry/delete", {
            key: history.key,
            expected_heads: history.heads,
          });
          return true;
        } finally {
          onBusy(false);
        }
      },
      (ok) => {
        if (ok) {
          client.notice("已移到回收站。");
          onChanged(history.key, true);
        }
      },
    );
  return (
    <>
      <Notice>
        {history.state === "conflict"
          ? "该条目存在多端同步冲突，请选择一个版本作为基础合并内容并保存。"
          : "选择一个历史版本即可查看或恢复为新版本，原有历史会保留。"}
      </Notice>
      {rules.blocked && <Notice>{rules.blocked}</Notice>}
      <div className="history-layout">
        <nav className="history-versions scroll-region" aria-label="历史版本">
          {history.versions.map((v) => (
            <button
              key={v.revision}
              className={
                "history-version" + (v.revision === selected ? " active" : "")
              }
              disabled={!v.available || task.busy || busy}
              onClick={() => {
                const choose = () => {
                  onDirty(false);
                  setSelected(v.revision);
                };
                if (!dirty) choose();
                else
                  void discard().then((ok) => {
                    if (ok) choose();
                  });
              }}
            >
              {v.deleted ? "删除记录" : v.head ? "当前分支" : "历史版本"} ·{" "}
              {v.revision.slice(0, 8)}
              <small>
                {v.key} · {groupLabel(v.group || "")}
              </small>
              <small>{new Date(v.updated_at).toLocaleString()}</small>
            </button>
          ))}
        </nav>
        <section className="history-editor">
          <ErrorNotice message={detail.error} />
          {detail.loading && <p>正在读取版本…</p>}
          {detail.data && (
            <RestoreEditor
              key={detail.data.revision}
              entry={detail.data}
              history={history}
              format={format}
              groups={groups}
              onDirty={onDirty}
              onBusy={onBusy}
              onChanged={onChanged}
            />
          )}
        </section>
      </div>
      {!selected && (
        <Notice>暂无可查看的历史版本，请等待同步完成或从备份恢复。</Notice>
      )}
      {history.state === "conflict" &&
        !rules.blocked &&
        !rules.groupConflict &&
        format >= 2 && (
          <Button
            variant="destructive"
            disabled={task.busy || busy}
            onClick={() => void remove()}
          >
            删除全部当前分支
          </Button>
        )}
      <ErrorNotice message={task.error} />
    </>
  );
}
function RestoreEditor({
  entry,
  history,
  format,
  groups,
  onDirty,
  onBusy,
  onChanged,
}: {
  entry: Entry;
  history: HistoryData;
  format: number;
  groups: Group[];
  onDirty: (value: boolean) => void;
  onBusy: (value: boolean) => void;
  onChanged: (key: string) => void;
}) {
  const rules = historyRules(history);
  const [target, setTarget] = useState(history.key),
    [group, setGroup] = useState(
      rules.groups.length === 1 ? rules.groups[0] : "",
    ),
    [groupConfirmed, setGroupConfirmed] = useState(false);
  const [description, setDescription] = useState(entry.description),
    [value, setValue] = useState(entry.value),
    [bodyFormat, setBodyFormat] = useState(entry.format),
    task = useTask(),
    confirm = useConfirm(),
    client = useClient();
  const reason = entry.deleted
    ? "删除记录不含正文，请选择删除前的内容版本。"
    : rules.blocked;
  const submit = () =>
    task.run(
      async (ctx) => {
        if (reason) throw new Error(reason);
        if (rules.groupConflict && !groupConfirmed)
          throw new Error("请明确确认恢复后的分组");
        if (
          !(await confirm({
            title: "保存恢复版本？",
            description:
              "将以当前内容和所选版本的附件保存为新版本。",
          }))
        )
          return false;
        onBusy(true);
        try {
          await ctx.request("/api/history/restore", {
            key: history.key,
            target_key: target,
            source_revision: entry.revision,
            expected_heads: history.heads,
            description,
            value,
            format: bodyFormat,
            ...(format >= 3 ? { group } : {}),
          });
          return true;
        } finally {
          onBusy(false);
        }
      },
      (ok) => {
        if (ok) {
          client.notice("已保存恢复版本。");
          onChanged(target);
        }
      },
    );
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        void submit();
      }}
      onChange={() => onDirty(true)}
    >
      <fieldset className="field-grid" disabled={task.busy || !!reason}>
        <Field id="restore-key" label="恢复后的 Key">
          <Input
            id="restore-key"
            value={target}
            readOnly={format < 2}
            required
            onChange={(e) => setTarget(e.target.value)}
          />
        </Field>
        <GroupField
          value={group}
          onChange={setGroup}
          groups={groups}
          format={format}
          id="restore-group"
        />
        {rules.groupConflict && (
          <label className="check-row">
            <input
              type="checkbox"
              checked={groupConfirmed}
              onChange={(e) => setGroupConfirmed(e.target.checked)}
              required
            />
            <span>
              分组存在分歧（{rules.groups.map(groupLabel).join("、")}
              ），我已明确选择恢复后的分组；留空为未分组。
            </span>
          </label>
        )}
        <Field id="restore-description" label="描述">
          <Textarea
            id="restore-description"
            value={description}
            required
            onChange={(e) => setDescription(e.target.value)}
          />
        </Field>
        <Field id="restore-value" label="历史正文（可编辑后合并）">
          <BodyEditor
            id="restore-value"
            label="历史正文（可编辑后合并）"
            value={value}
            format={bodyFormat}
            disabled={task.busy || !!reason}
            onChange={(value) => {
              setValue(value);
              onDirty(true);
            }}
          />
        </Field>
        <ChoiceField
          id="restore-format"
          label="正文格式"
          value={bodyFormat}
          options={formatChoices}
          onChange={setBodyFormat}
        />
        <Button type="submit" disabled={task.busy || !!reason}>
          {task.busy
            ? "正在保存…"
            : history.state === "conflict"
              ? "保存合并版本"
              : "恢复为新版本"}
        </Button>
      </fieldset>
      {reason && <Notice>{reason}</Notice>}
      <ErrorNotice message={task.error} />
      <Attachments files={entry.attachments || []} />
    </form>
  );
}
