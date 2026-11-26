import { useState } from "react";
import { ArrowRight, FolderClosed, LockKeyhole } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Brand, Field } from "@/components/common";
import { ErrorNotice, Modal, Notice, useConfirm } from "@/components/forms";
import { useTask } from "@/lib/hooks";
import { readFile } from "@/lib/files";
import { type AppState, type VaultAction, vaultName } from "@/lib/types";

export function validatePassword(value: string, repeat: string) {
  if (value !== repeat) throw new Error("两次输入的主密码不一致");
  if ([...value].length < 12 || new TextEncoder().encode(value).length > 1024)
    throw new Error("主密码至少 12 个字符，最多 1024 字节");
}
export function Unlock({ state }: { state: AppState }) {
  const [password, setPassword] = useState(""),
    [flow, setFlow] = useState<VaultAction | null>(null),
    task = useTask();
  const blocked =
    state.system.enabled && (state.system.locked || state.system.sleeping);
  return (
    <main className="lock-screen">
      <header className="lock-topbar">
        <Brand />
      </header>
      <div className="lock-center scroll-region">
        <section className="unlock-card">
          <div className="vault-emblem">
            <LockKeyhole />
          </div>
          <h1>{state.vault.exists ? "解锁资料库" : "创建资料库"}</h1>
          <div className="unlock-vault">
            <FolderClosed />
            <span title={state.vault.directory}>
              {vaultName(state.vault.directory)}
            </span>
          </div>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void task.run(async (ctx) => {
                try {
                  await ctx.request("/api/vault/unlock", { password });
                  await ctx.refresh();
                } finally {
                  setPassword("");
                }
              });
            }}
          >
            <fieldset disabled={task.busy || blocked}>
              {state.vault.exists ? (
                <>
                  <Field id="unlock-password" label="主密码">
                    <Input
                      id="unlock-password"
                      type="password"
                      autoComplete="current-password"
                      required
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      autoFocus
                    />
                  </Field>
                  <Button className="unlock-submit" type="submit">
                    {task.busy ? "正在解锁…" : "解锁"}
                    <ArrowRight />
                  </Button>
                </>
              ) : (
                <>
                  <p className="field-note">此目录尚无资料库。</p>
                  <Button
                    className="unlock-submit"
                    type="button"
                    onClick={() => setFlow("create")}
                  >
                    设置主密码
                    <ArrowRight />
                  </Button>
                </>
              )}
            </fieldset>
            <ErrorNotice message={task.error} />
          </form>
          {state.system.enabled && (!state.system.available || blocked) && (
            <Notice>
              {state.system.message || "系统会话仍锁定或休眠，请先解锁系统。"}
            </Notice>
          )}
          {state.system.lock_reason && (
            <p className="field-note">{state.system.lock_reason}</p>
          )}
          {state.device_notice && <Notice>{state.device_notice}</Notice>}
          <div className="unlock-links">
            <button onClick={() => setFlow("open")}>打开其他资料库</button>
            {state.vault.exists && (
              <button onClick={() => setFlow("create")}>创建资料库</button>
            )}
            <button onClick={() => setFlow("restore")}>从备份恢复</button>
          </div>
          <p className="lock-caption path-value">{state.vault.directory}</p>
        </section>
      </div>
      {flow && (
        <VaultFlow kind={flow} state={state} onClose={() => setFlow(null)} />
      )}
    </main>
  );
}
export function VaultFlow({
  kind,
  state,
  onClose,
}: {
  kind: VaultAction;
  state: AppState;
  onClose: () => void;
}) {
  const createHere = kind === "create" && !state.vault.exists;
  const [directory, setDirectory] = useState(
    kind === "restore"
      ? state.vault.directory + "-restored"
      : createHere
        ? state.vault.directory
        : "",
  );
  const [password, setPassword] = useState(""),
    [repeat, setRepeat] = useState(""),
    [file, setFile] = useState<File | null>(null),
    task = useTask(),
    confirm = useConfirm();
  const title =
    kind === "restore"
      ? "从备份恢复"
      : createHere
        ? "设置主密码"
        : kind === "create"
          ? "选择新资料库目录"
          : "打开其他资料库";
  const submit = () =>
    task.run(
      async (ctx) => {
        try {
          if (kind === "restore") {
            if (!file) throw new Error("请选择备份 ZIP");
            if (
              !(await confirm({
                title: "恢复并切换资料库？",
                description:
                  "先校验备份，再恢复到新目录或空目录。不会覆盖现有资料，完成后需要重新解锁。",
              }))
            )
              return;
            const archive = await readFile(file, 66 * 1024 * 1024, ctx.assert);
            await ctx.request("/api/vault/restore", {
              directory,
              password,
              archive,
            });
            await ctx.refresh("备份已恢复，请使用备份的主密码解锁。");
          } else if (createHere) {
            validatePassword(password, repeat);
            await ctx.request("/api/vault/create", { password });
            await ctx.refresh("资料库已创建。");
          } else {
            if (
              state.authenticated &&
              !(await confirm({
                title: "切换资料库？",
                description: "当前资料库将锁定，未完成的 AI 请求会失效。",
              }))
            )
              return;
            await ctx.request("/api/vault/select", { directory });
            await ctx.refresh(
              kind === "create"
                ? "目录已选定，可设置主密码创建资料库。"
                : "已切换资料库。",
            );
          }
          return true;
        } finally {
          setPassword("");
          setRepeat("");
        }
      },
      (completed) => {
        if (completed) onClose();
      },
    );
  return (
    <Modal
      title={title}
      busy={task.busy}
      onClose={onClose}
      footer={
        <>
          <Button variant="outline" disabled={task.busy} onClick={onClose}>
            取消
          </Button>
          <Button type="submit" form="vault-flow" disabled={task.busy}>
            {task.busy
              ? "正在处理…"
              : kind === "restore"
                ? "验证并恢复"
                : createHere
                  ? "创建资料库"
                  : "选择目录"}
          </Button>
        </>
      }
    >
      <form
        id="vault-flow"
        onSubmit={(e) => {
          e.preventDefault();
          void submit();
        }}
      >
        <fieldset disabled={task.busy} className="field-grid">
          {kind === "restore" && (
            <>
              <Notice>
                仅恢复到新目录或空目录。备份含公开元信息与加密内容，不含本机接入凭据。
              </Notice>
              <Field id="backup-file" label="备份 ZIP" note="最多 66 MiB">
                <Input
                  id="backup-file"
                  type="file"
                  accept=".zip,application/zip"
                  required
                  onChange={(e) => setFile(e.target.files?.[0] || null)}
                />
              </Field>
            </>
          )}
          <Field
            id="vault-directory"
            label="资料库目录"
            note={
              createHere
                ? undefined
                : "请输入绝对路径；本机配置目录不能放在资料库内。"
            }
          >
            <Input
              id="vault-directory"
              autoComplete="username"
              className="path-value"
              value={directory}
              readOnly={createHere}
              required
              onChange={(e) => setDirectory(e.target.value)}
              spellCheck={false}
            />
          </Field>
          {(createHere || kind === "restore") && (
            <Field
              id="vault-password"
              label={createHere ? "主密码" : "备份的主密码"}
              note={
                createHere
                  ? "至少 12 个字符。忘记密码且没有有效恢复材料时无法恢复。"
                  : undefined
              }
            >
              <Input
                id="vault-password"
                type="password"
                autoComplete={createHere ? "new-password" : "current-password"}
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </Field>
          )}
          {createHere && (
            <Field id="vault-repeat" label="再次输入主密码">
              <Input
                id="vault-repeat"
                type="password"
                autoComplete="new-password"
                required
                value={repeat}
                onChange={(e) => setRepeat(e.target.value)}
              />
            </Field>
          )}
        </fieldset>
        <ErrorNotice message={task.error} />
      </form>
    </Modal>
  );
}
