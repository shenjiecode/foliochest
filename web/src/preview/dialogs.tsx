import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";
import { Field, SelectField } from "./shared";
import { groupsOf, type DialogKind, type Entry } from "./model";
const titles: Record<Exclude<DialogKind, null>, string> = {
  new: "新建条目",
  move: "移动到分组",
  rename: "重命名 Key",
  delete: "移到回收站？",
  history: "历史版本",
  "open-vault": "打开资料库",
  "create-vault": "创建资料库",
  "restore-vault": "从备份恢复",
  password: "修改主密码",
};
export function Dialogs({
  kind,
  entries,
  current,
  checked,
  onClose,
  onEnter,
  notice,
  onSave,
  onCreate,
  onMove,
  onDelete,
}: {
  kind: DialogKind;
  entries: Entry[];
  current?: Entry;
  checked: Set<string>;
  onClose: () => void;
  onEnter: () => void;
  notice: (text: string) => void;
  onSave: (entry: Entry) => void;
  onCreate: (entry: Entry) => void;
  onMove: (group: string) => void;
  onDelete: () => void;
}) {
  const [name, setName] = useState(kind === "rename" ? current?.key || "" : "");
  const [description, setDescription] = useState("");
  const [group, setGroup] = useState("");
  const [contentType, setContentType] = useState<Entry["type"]>("Markdown");
  const [step, setStep] = useState(0);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(
    Math.max(0, (current?.history.length || 1) - 1),
  );
  if (!kind) return null;
  const title = titles[kind];
  const historical = current?.history[revision];
  const submitName = () => {
    const key = name.trim();
    if (!key) {
      setError("请填写 Key。");
      return;
    }
    if (
      entries.some(
        (entry) =>
          entry.key === key && (kind !== "rename" || entry.id !== current?.id),
      )
    ) {
      setError("这个 Key 已被条目或回收站占用。");
      return;
    }
    if (kind === "rename" && current) {
      onSave({ ...current, key });
      onClose();
      return;
    }
    const id = "example-" + Date.now();
    onCreate({
      id,
      key,
      description,
      group,
      type: contentType,
      value: "",
      attachments: [],
      history: [
        { id: id + "-1", label: "创建示例", value: "", attachments: [] },
      ],
      read: "ask",
      write: "ask",
      deleted: false,
    });
  };
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent
        className={cn("dialog-card", kind === "history" && "history-dialog")}
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription className="sr-only">
            界面预览；所有操作只影响当前页面中的示例。
          </DialogDescription>
        </DialogHeader>
        <div className="dialog-body">
          {(kind === "new" || kind === "rename") && (
            <div className="field-grid">
              <Field id="dialog-key" label="Key">
                <Input
                  id="dialog-key"
                  autoFocus
                  value={name}
                  maxLength={256}
                  onChange={(event) => {
                    setName(event.target.value);
                    setError("");
                  }}
                  placeholder="例如：项目/交付说明"
                  onKeyDown={(event) => {
                    if (event.key === "Enter") submitName();
                  }}
                />
              </Field>
              {kind === "new" && (
                <>
                  <Field id="new-description" label="描述">
                    <Input
                      id="new-description"
                      value={description}
                      onChange={(event) => setDescription(event.target.value)}
                      placeholder="这条资料包含什么"
                    />
                  </Field>
                  <div className="field-pair">
                    <Field id="new-group" label="分组">
                      <SelectField
                        id="new-group"
                        value={group}
                        onChange={(event) => setGroup(event.target.value)}
                      >
                        <option value="">未分组</option>
                        {groupsOf(entries).map((name) => (
                          <option key={name}>{name}</option>
                        ))}
                      </SelectField>
                    </Field>
                    <Field id="new-type" label="内容格式">
                      <SelectField
                        id="new-type"
                        value={contentType}
                        onChange={(event) =>
                          setContentType(event.target.value as Entry["type"])
                        }
                      >
                        <option>Markdown</option>
                        <option>文本</option>
                        <option>JSON</option>
                      </SelectField>
                    </Field>
                  </div>
                </>
              )}
            </div>
          )}
          {kind === "move" && (
            <div className="field-grid">
              <p className="field-note">
                移动 {checked.size} 条资料；不改变 Key 或 AI 权限。
              </p>
              <ul className="selected-keys">
                {entries
                  .filter((entry) => checked.has(entry.id))
                  .map((entry) => (
                    <li key={entry.id}>{entry.key}</li>
                  ))}
              </ul>
              <Field id="move-group" label="目标分组">
                <SelectField
                  id="move-group"
                  value={group}
                  onChange={(event) => setGroup(event.target.value)}
                >
                  <option value="">未分组</option>
                  {groupsOf(entries).map((name) => (
                    <option key={name}>{name}</option>
                  ))}
                </SelectField>
              </Field>
            </div>
          )}
          {kind === "delete" && (
            <div className="field-grid">
              <p className="font-medium break-all">{current?.key}</p>
              <p className="field-note">
                可以从回收站恢复。此操作不清除历史，也不会擦除旧副本。
              </p>
            </div>
          )}
          {kind === "history" && (
            <div className="history-layout">
              <nav className="history-versions" aria-label="选择历史版本">
                {current?.history.map((item, index) => (
                  <button
                    key={item.id}
                    className={cn(
                      "history-version",
                      revision === index && "active",
                    )}
                    onClick={() => setRevision(index)}
                    aria-current={revision === index ? "true" : undefined}
                  >
                    版本 {index + 1}
                    <small>{item.label}</small>
                  </button>
                ))}
              </nav>
              <pre className="history-body">
                {historical?.value || "这个版本的正文为空。"}
              </pre>
            </div>
          )}
          {kind === "open-vault" && (
            <div className="field-grid">
              <Field id="open-directory" label="资料库目录">
                <Input
                  id="open-directory"
                  readOnly
                  value="/示例资料库/个人资料"
                  className="path-value"
                />
              </Field>
              <p className="field-note">预览不会读取这个目录。</p>
            </div>
          )}
          {kind === "create-vault" && (
            <div className="field-grid">
              {step === 0 ? (
                <Field id="create-directory" label="新资料库目录">
                  <Input
                    id="create-directory"
                    readOnly
                    value="/示例资料库/新资料库"
                    className="path-value"
                  />
                </Field>
              ) : (
                <>
                  <Field id="create-password" label="主密码">
                    <Input
                      id="create-password"
                      type="password"
                      readOnly
                      autoComplete="off"
                      value=""
                      placeholder="预览无需输入"
                    />
                  </Field>
                  <Field id="create-password-confirm" label="确认主密码">
                    <Input
                      id="create-password-confirm"
                      type="password"
                      readOnly
                      autoComplete="off"
                      value=""
                      placeholder="预览无需输入"
                    />
                  </Field>
                </>
              )}
            </div>
          )}
          {kind === "restore-vault" && (
            <>
              <p className="dialog-notice">
                恢复到新目录或空目录，不覆盖现有资料库。完成后需要重新解锁。
              </p>
              <div className="field-grid">
                {step === 0 ? (
                  <>
                    <Field id="backup-file" label="备份文件">
                      <Input
                        id="backup-file"
                        readOnly
                        value="foliochest-example.zip"
                      />
                    </Field>
                    <Field id="restore-directory" label="恢复到">
                      <Input
                        id="restore-directory"
                        readOnly
                        value="/示例资料库/恢复的资料"
                        className="path-value"
                      />
                    </Field>
                  </>
                ) : (
                  <Field id="backup-password" label="备份的主密码">
                    <Input
                      id="backup-password"
                      type="password"
                      readOnly
                      autoComplete="off"
                      value=""
                      placeholder="预览无需输入"
                    />
                  </Field>
                )}
              </div>
            </>
          )}
          {kind === "password" && (
            <>
              <p className="dialog-notice">
                修改后将锁定资料库，并关闭本机自动解锁。旧备份仍使用旧密码。
              </p>
              <div className="field-grid">
                {[
                  ["old-password", "当前主密码"],
                  ["new-password", "新主密码"],
                  ["confirm-password", "确认新主密码"],
                ].map(([id, label]) => (
                  <Field key={id} id={id} label={label}>
                    <Input
                      id={id}
                      type="password"
                      readOnly
                      autoComplete="off"
                      value=""
                      placeholder="预览不接收密码"
                    />
                  </Field>
                ))}
              </div>
            </>
          )}
          {error && (
            <p className="form-error" role="alert">
              {error}
            </p>
          )}
        </div>
        <DialogFooter>
          <span className="footer-note">仅示例</span>
          {(kind === "create-vault" || kind === "restore-vault") && step > 0 ? (
            <Button variant="outline" onClick={() => setStep(0)}>
              上一步
            </Button>
          ) : (
            <Button variant="outline" onClick={onClose}>
              取消
            </Button>
          )}
          {(kind === "new" || kind === "rename") && (
            <Button onClick={submitName}>
              {kind === "new" ? "创建条目" : "保存"}
            </Button>
          )}
          {kind === "move" && (
            <Button disabled={!checked.size} onClick={() => onMove(group)}>
              移动
            </Button>
          )}
          {kind === "delete" && (
            <Button variant="destructive" onClick={onDelete}>
              移到回收站
            </Button>
          )}
          {kind === "history" && (
            <Button
              disabled={!historical || !current}
              onClick={() => {
                if (historical && current) {
                  onSave({
                    ...current,
                    value: historical.value,
                    attachments: historical.attachments.map((file) => ({
                      ...file,
                    })),
                    deleted: false,
                  });
                  onClose();
                  notice("示例正文已恢复为新版本，保留当前名称、分组与权限。");
                }
              }}
            >
              恢复此版本
            </Button>
          )}
          {kind === "open-vault" && (
            <Button
              onClick={() => {
                onEnter();
                notice("已进入示例工作区，未打开本机资料库。");
              }}
            >
              打开
            </Button>
          )}
          {kind === "create-vault" && (
            <Button
              onClick={() => {
                if (!step) setStep(1);
                else {
                  onEnter();
                  notice("创建流程演示结束，未创建资料库。");
                }
              }}
            >
              {step === 0 ? "下一步" : "创建资料库"}
            </Button>
          )}
          {kind === "restore-vault" && (
            <Button
              onClick={() => {
                if (!step) setStep(1);
                else {
                  onClose();
                  notice("恢复流程演示结束，未读取或写入文件。");
                }
              }}
            >
              {step === 0 ? "下一步" : "恢复资料库"}
            </Button>
          )}
          {kind === "password" && <Button disabled>修改主密码</Button>}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
