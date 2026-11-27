import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field } from "@/components/common";
import {
  ErrorNotice,
  GroupField,
  Modal,
  Notice,
  useConfirm,
} from "@/components/forms";
import { useClient, useTask } from "@/lib/hooks";
import type { Entry, Group, Member } from "@/lib/types";
import { groupLabel } from "@/lib/groups";
export function EntryManagement({
  kind,
  entry,
  onClose,
  onChanged,
}: {
  kind: "rename" | "delete";
  entry: Entry;
  onClose: () => void;
  onChanged: (key: string, deleted: boolean) => void;
}) {
  const [target, setTarget] = useState(entry.key),
    task = useTask(),
    client = useClient();
  return (
    <Modal
      title={kind === "delete" ? "移到回收站？" : "重命名 Key"}
      busy={task.busy}
      onClose={onClose}
      footer={
        <>
          <Button variant="outline" disabled={task.busy} onClick={onClose}>
            取消
          </Button>
          <Button
            variant={kind === "delete" ? "destructive" : "default"}
            form="manage-entry"
            type="submit"
            disabled={task.busy}
          >
            {task.busy
              ? "处理中…"
              : kind === "delete"
                ? "移到回收站"
                : "确认重命名"}
          </Button>
        </>
      }
    >
      <form
        id="manage-entry"
        onSubmit={(e) => {
          e.preventDefault();
          void task.run(
            async (ctx) => {
              if (kind === "rename" && target === entry.key)
                throw new Error("新 Key 与当前名称相同");
              await ctx.request(
                kind === "delete" ? "/api/entry/delete" : "/api/entry/rename",
                kind === "delete"
                  ? { key: entry.key, expected_heads: [entry.revision] }
                  : {
                      key: entry.key,
                      expected_revision: entry.revision,
                      new_key: target,
                    },
              );
            },
            () => {
              onChanged(
                kind === "delete" ? entry.key : target,
                kind === "delete",
              );
              client.notice(
                kind === "delete"
                  ? "已移到回收站，历史和附件保留。"
                  : "Key 已重命名。",
              );
              onClose();
            },
          );
        }}
      >
        <Notice>
          {kind === "delete"
            ? "删除会保留历史、附件和名称占用，可从删除前的版本恢复。这不是安全擦除，也不会释放磁盘空间。"
            : "正文、附件、分组和权限不变。旧 Key 不再指向此条目，旧审批会失效。"}
        </Notice>
        <p className="path-value">{entry.key}</p>
        {kind === "rename" && (
          <Field id="rename-key" label="新 Key">
            <Input
              id="rename-key"
              value={target}
              required
              disabled={task.busy}
              onChange={(e) => setTarget(e.target.value)}
              spellCheck={false}
            />
          </Field>
        )}
        <ErrorNotice message={task.error} />
      </form>
    </Modal>
  );
}
export function GroupManagement({
  members,
  source,
  groups,
  format,
  onClose,
  onChanged,
}: {
  members: Member[];
  source?: Group;
  groups: Group[];
  format: number;
  onClose: () => void;
  onChanged: (group: string | null) => void;
}) {
  const [reviewed] = useState(() =>
    source
      ? { ...source, members: source.members.map((m) => ({ ...m })) }
      : null,
  );
  const [selection] = useState(() => members.map((m) => ({ ...m }))),
    [target, setTarget] = useState(source?.name || ""),
    task = useTask(),
    confirm = useConfirm(),
    client = useClient();
  const submit = (remove = false) =>
    task.run(
      async (ctx) => {
        if (format < 3) throw new Error("分组需要格式 3");
        const entries = reviewed?.members || selection;
        if ((!reviewed && !entries.length) || entries.length > 20)
          throw new Error("每次可管理 1–20 条，请手动分批移动");
        if (reviewed?.blocked) throw new Error("组内有冲突或缺失，请先处理");
        const next = remove ? "" : target;
        if (reviewed && !remove && !next)
          throw new Error("请输入新组名，或选择解除分组");
        if (reviewed && next === reviewed.name)
          throw new Error("新组名与当前相同");
        if (
          !(await confirm({
            title: remove ? "解除分组？" : "确认移动分组？",
            description: `将 ${entries.length} 条资料移到「${groupLabel(next)}」。${reviewed ? `包含回收站 ${reviewed.deleted_count} 条。` : ""}正文、附件、Key 和权限不变，旧审批会失效。`,
          }))
        )
          return;
        await ctx.request(
          reviewed ? "/api/groups/edit" : "/api/groups/move",
          reviewed
            ? { source: reviewed.name, target: next, expected: entries }
            : { group: next, entries },
        );
        return next;
      },
      (next) => {
        if (next === undefined) return;
        onChanged(next || null);
        client.notice("分组已更新。");
        onClose();
      },
    );
  return (
    <Modal
      title={source ? "管理分组" : "移动到分组"}
      busy={task.busy}
      onClose={onClose}
      footer={
        <>
          <Button variant="outline" onClick={onClose} disabled={task.busy}>
            取消
          </Button>
          {source && (
            <Button
              variant="outline"
              disabled={task.busy || !!reviewed?.blocked}
              onClick={() => void submit(true)}
            >
              解除分组
            </Button>
          )}
          <Button
            form="group-form"
            type="submit"
            disabled={task.busy || !!reviewed?.blocked}
          >
            {task.busy ? "处理中…" : source ? "重命名分组" : "移动"}
          </Button>
        </>
      }
    >
      <form
        id="group-form"
        onSubmit={(e) => {
          e.preventDefault();
          void submit();
        }}
      >
        <fieldset disabled={task.busy} className="field-grid">
          <Notice>
            {source
              ? `${source.count} 条资料，${source.deleted_count} 条在回收站。整组操作绑定此时查看的完整成员及版本。`
              : "每次最多 20 条，超限整体拒绝，不自动拆批。"}
            {reviewed?.blocked ? " 组内有不可用条目，请先处理。" : ""}
          </Notice>
          <div className="selected-keys">
            {(reviewed?.members || selection).map((m) => (
              <span key={m.key}>{m.key}</span>
            ))}
          </div>
          <GroupField
            value={target}
            onChange={setTarget}
            groups={groups}
            format={format}
            id="target-group"
          />
        </fieldset>
        <ErrorNotice message={task.error} />
      </form>
    </Modal>
  );
}
