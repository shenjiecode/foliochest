import { useState } from "react";
import { ArrowRight, FolderClosed, FolderOpen } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Brand, Field } from "@/components/common";
import { ErrorNotice, Modal, Notice, useConfirm } from "@/components/forms";
import { useTask } from "@/lib/hooks";
import { useQuit } from "@/components/exit-app";
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
    task = useTask(),
    exit = useQuit();
  const selected = !!state.vault.directory;
  const exists = state.vault.exists;
  const configured = state.configured ?? (selected || exists);
  const blocked =
    state.system.enabled && (state.system.locked || state.system.sleeping);
  return (
    <main className="lock-screen">
      <header className="lock-topbar">
        <Brand />
        <Button
          variant="ghost"
          disabled={task.busy || exit.busy}
          onClick={() => void exit.quit()}
        >
          退出资料匣
        </Button>
      </header>
      <ErrorNotice message={exit.error} />
      <div className="lock-center scroll-region">
        <section className="unlock-card">
          <div className="vault-emblem">
            <img
              src="/logo.png"
              width="88"
              height="88"
              alt="资料匣 FolioChest"
            />
          </div>
          <h1>
            {exists
              ? "解锁资料匣"
              : configured
                ? "找不到资料文件"
                : "开始使用资料匣"}
          </h1>
          {!configured && (
            <p className="field-note">
              本地加密资料库，设置一个主密码即可开始。
            </p>
          )}
          {selected && (
            <div className="unlock-vault">
              <FolderClosed />
              <span title={state.vault.directory}>
                {vaultName(state.vault.directory)}
              </span>
            </div>
          )}
          {exists ? (
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
              </fieldset>
              <ErrorNotice message={task.error} />
            </form>
          ) : (
            <div className="onboarding-actions">
              {selected && (
                <p className="field-note">
                  原位置没有找到资料库，请检查目录、权限或同步状态。
                </p>
              )}
              <Button onClick={() => setFlow(configured ? "open" : "create")}>
                {configured ? "重新定位原资料库" : "设置主密码并开始"}
                <ArrowRight />
              </Button>
              {!configured && (
                <Button variant="outline" onClick={() => setFlow("open")}>
                  接入已有资料库
                </Button>
              )}
              {configured && (
                <Button
                  variant="outline"
                  disabled={task.busy}
                  onClick={() => void task.run((ctx) => ctx.refresh())}
                >
                  重新检查原位置
                </Button>
              )}
            </div>
          )}
          {state.system.enabled &&
            (blocked || (selected && !state.system.available)) && (
              <Notice>
                {blocked
                  ? "系统会话仍处于锁定或休眠状态，请先解锁系统。"
                  : state.system.message}
              </Notice>
            )}
          {state.system.lock_reason && (
            <p className="field-note">{state.system.lock_reason}</p>
          )}
          {state.device_notice && <Notice>{state.device_notice}</Notice>}
          <div className="unlock-links">
            <button disabled={task.busy} onClick={() => setFlow("restore")}>
              从备份恢复
            </button>
          </div>
          {selected && (
            <p className="lock-caption path-value">{state.vault.directory}</p>
          )}
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
  const [directory, setDirectory] = useState("");
  const [stage, setStage] = useState<"directory" | "password">("directory");
  const [password, setPassword] = useState(""),
    [repeat, setRepeat] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const task = useTask(),
    picker = useTask(),
    confirm = useConfirm();
  const busy = task.busy || picker.busy;
  const creating = kind === "create";
  const migrating = kind === "migrate";
  const passwordStep = creating && stage === "password";
  const title = migrating
    ? "迁移保存位置"
    : kind === "restore"
      ? "恢复资料库备份"
      : passwordStep
        ? "设置主密码"
        : creating
          ? "选择新资料库目录"
          : state.configured
            ? "重新定位原资料库"
            : "接入已有资料库";
  const browse = () =>
    picker.run(
      async (ctx) => {
        return ctx.request<{ cancelled: boolean; directory?: string }>(
          "/api/directory/pick",
          { directory },
        );
      },
      (result) => {
        if (!result.cancelled && result.directory) {
          setDirectory(result.directory);
          task.clearError();
        }
      },
    );
  const back = () => {
    setPassword("");
    setRepeat("");
    setStage("directory");
    task.clearError();
  };
  const submit = () => {
    if (busy) return;
    if (creating && !passwordStep) {
      if (directory.trim()) {
        setStage("password");
        task.clearError();
        picker.clearError();
      }
      return;
    }
    return task.run(
      async (ctx) => {
        try {
          if (creating) validatePassword(password, repeat);
          if (kind === "restore" && !file) throw new Error("请选择备份 ZIP");
          if (kind === "restore" || state.authenticated) {
            if (
              !(await confirm({
                title: kind === "restore" ? "从备份恢复？" : "迁移保存位置？",
                description:
                  kind === "restore"
                    ? "将备份恢复到所选空目录，原目录保持不变，完成后需使用备份密码重新解锁。"
                    : "将资料库复制到新目录并切换使用，原目录保持不变，完成后需重新解锁。",
              }))
            )
              return false;
          }
          if (migrating) {
            await ctx.request("/api/vault/migrate", {
              directory,
              password,
              confirmed: true,
            });
            await ctx.refresh("已迁移保存位置，请重新解锁。");
          } else if (kind === "restore") {
            const archive = await readFile(file!, 66 * 1024 * 1024, ctx.assert);
            await ctx.request("/api/vault/restore", {
              directory,
              password,
              archive,
              confirmed: true,
            });
            await ctx.refresh("备份已恢复，请使用备份的主密码解锁。");
          } else if (creating) {
            await ctx.request("/api/vault/create", { directory, password });
            await ctx.refresh("资料库已创建。");
          } else {
            await ctx.request("/api/vault/select", { directory, password });
            await ctx.refresh("资料匣已解锁。");
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
  };
  return (
    <Modal
      title={title}
      busy={busy}
      onClose={onClose}
      footer={
        <>
          <Button
            variant="outline"
            disabled={busy}
            onClick={passwordStep ? back : onClose}
          >
            {passwordStep ? "上一步" : "取消"}
          </Button>
          <Button type="submit" form="vault-flow" disabled={busy}>
            {task.busy
              ? "正在处理…"
              : kind === "restore"
                ? "验证并恢复"
                : passwordStep
                  ? "创建资料库"
                  : creating
                    ? "下一步"
                    : migrating
                      ? "验证并迁移"
                      : "验证并解锁"}
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
        <fieldset disabled={busy} className="field-grid">
          {migrating && (
            <Notice>迁移会保留原目录，完成后需重新解锁。</Notice>
          )}
          {kind === "restore" && (
            <Field id="backup-file" label="备份 ZIP" note="最多 66 MiB">
              <Input
                id="backup-file"
                type="file"
                accept=".zip,application/zip"
                required
                onChange={(e) => setFile(e.target.files?.[0] || null)}
              />
            </Field>
          )}
          {passwordStep ? (
            <div className="chosen-directory">
              <span>保存位置</span>
              <p className="path-value">{directory}</p>
            </div>
          ) : (
            <Field
              id="vault-directory"
              label="资料库目录"
              note={
                creating || kind === "restore" || migrating
                  ? "选择新文件夹或空文件夹"
                  : "选择包含 vault.json 的文件夹"
              }
            >
              <div className="directory-input">
                <Input
                  id="vault-directory"
                  className="path-value"
                  value={directory}
                  placeholder="选择文件夹，或输入绝对路径"
                  required
                  onChange={(e) => {
                    setDirectory(e.target.value);
                    task.clearError();
                    picker.clearError();
                  }}
                  spellCheck={false}
                  autoComplete="off"
                />
                <Button
                  type="button"
                  variant="outline"
                  disabled={busy}
                  onClick={() => void browse()}
                >
                  <FolderOpen />
                  {picker.busy ? "等待选择…" : "选择文件夹"}
                </Button>
              </div>
              {picker.busy && (
                <p className="field-note" role="status">
                  请在系统窗口中选择文件夹。
                </p>
              )}
              <ErrorNotice message={picker.error} />
            </Field>
          )}
          {(passwordStep || !creating) && (
            <Field
              id="vault-password"
              label={kind === "restore" ? "备份的主密码" : "主密码"}
              note={
                creating
                  ? "至少 12 个字符，请牢记主密码，忘记后无法找回。"
                  : undefined
              }
            >
              <Input
                id="vault-password"
                type="password"
                autoComplete={creating ? "new-password" : "current-password"}
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoFocus={passwordStep}
              />
            </Field>
          )}
          {passwordStep && (
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
