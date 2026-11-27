import { useState } from "react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Field, SettingRow } from "@/components/common";
import { ErrorNotice, Modal, Notice, useConfirm } from "@/components/forms";
import { useClient, useResource, useTask } from "@/lib/hooks";
import { download } from "@/lib/files";
import { type AppState, type Security, type VaultAction } from "@/lib/types";
import { DeviceSettings } from "./settings-device";
import { MCPSettings } from "./settings-mcp";
import { UpdateSettings } from "./settings-update";
import { validatePassword } from "./unlock";
export function Settings({
  state,
  onOpen,
}: {
  state: AppState;
  onOpen: (kind: VaultAction) => void;
}) {
  const [dialog, setDialog] = useState<"password" | "upgrade" | null>(null),
    task = useTask(),
    client = useClient();
  return (
    <section className="settings-shell" aria-label="设置">
      <header className="page-heading">
        <h1>设置</h1>
      </header>
      <Tabs defaultValue="vault" className="settings-tabs">
        <TabsList>
          <TabsTrigger value="vault">资料库</TabsTrigger>
          <TabsTrigger value="security">安全</TabsTrigger>
          <TabsTrigger value="device">设备</TabsTrigger>
          <TabsTrigger value="mcp">MCP</TabsTrigger>
        </TabsList>
        <TabsContent value="vault" className="settings-body">
          <section className="settings-section">
            <SettingRow title="保存位置" note={state.vault.directory}>
              <Button variant="outline" onClick={() => onOpen("migrate")}>
                迁移位置
              </Button>
            </SettingRow>
            <SettingRow title="资料库格式" note={String(state.vault.format)}>
              {state.vault.format < 3 ? (
                <Button variant="outline" onClick={() => setDialog("upgrade")}>
                  升级为格式 3
                </Button>
              ) : (
                <span>当前格式</span>
              )}
            </SettingRow>
            <SettingRow
              title="加密备份"
              note="包含公开元信息、加密正文、附件和全部历史，不含本机凭据。"
            >
              <Button
                variant="outline"
                disabled={task.busy}
                onClick={() =>
                  void task.run(
                    (ctx) => ctx.blob("/api/backup", {}),
                    (blob) => {
                      download(
                        blob,
                        "foliochest-backup-" +
                          new Date().toISOString().slice(0, 10) +
                          ".zip",
                      );
                      client.notice("备份已生成，请保留下载文件与主密码。");
                    },
                  )
                }
              >
                {task.busy ? "正在生成…" : "下载备份"}
              </Button>
            </SettingRow>
            <ErrorNotice message={task.error} />
            <SettingRow
              title="从备份恢复"
              note="仅恢复本库备份；校验后使用恢复目录，原目录保留，不覆盖已有资料。恢复后需重新解锁。"
            >
              <Button variant="outline" onClick={() => onOpen("restore")}>
                恢复
              </Button>
            </SettingRow>
            <UpdateSettings version={state.version} />
          </section>
        </TabsContent>
        <TabsContent value="security" className="settings-body">
          <SecuritySettings state={state} />
          <section className="settings-section">
            <SettingRow
              title="主密码"
              note="进入资料匣与解锁资料使用同一个密码。变更后锁定并关闭本机设备授权。"
            >
              <Button variant="outline" onClick={() => setDialog("password")}>
                变更主密码
              </Button>
            </SettingRow>
          </section>
        </TabsContent>
        <TabsContent value="device" className="settings-body">
          <DeviceSettings />
        </TabsContent>
        <TabsContent value="mcp" className="settings-body">
          <MCPSettings />
        </TabsContent>
      </Tabs>
      {dialog && (
        <PasswordDialog
          kind={dialog}
          format={state.vault.format}
          onClose={() => setDialog(null)}
        />
      )}
    </section>
  );
}
function SecuritySettings({ state }: { state: AppState }) {
  const resource = useResource<Security>("/api/settings");
  return (
    <section className="settings-section">
      <ErrorNotice message={resource.error} />
      {resource.loading ? (
        <p>正在读取设置…</p>
      ) : (
        resource.data && <SecurityForm initial={resource.data} state={state} />
      )}
    </section>
  );
}
function SecurityForm({
  initial,
  state,
}: {
  initial: Security;
  state: AppState;
}) {
  const [minutes, setMinutes] = useState(String(initial.idle_lock_minutes)),
    [system, setSystem] = useState(initial.lock_on_system_events),
    task = useTask(),
    client = useClient();
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        void task.run(
          async (ctx) => {
            const n = Number(minutes);
            if (!Number.isInteger(n) || n < 0 || n > 1440)
              throw new Error("空闲时间应为 0–1440 分钟");
            await ctx.request("/api/settings", {
              idle_lock_minutes: n,
              lock_on_system_events: system,
            });
            await ctx.refresh();
          },
          () => client.notice("锁定设置已保存。"),
        );
      }}
    >
      <fieldset disabled={task.busy}>
        <SettingRow
          title="网页空闲锁定"
          note="0 为关闭。只计算网页操作，轮询和 MCP 不续期。"
        >
          <Input
            aria-label="空闲锁定分钟"
            type="number"
            min={0}
            max={1440}
            required
            value={minutes}
            onChange={(e) => setMinutes(e.target.value)}
            className="minutes-input"
          />
        </SettingRow>
        <SettingRow
          title="系统事件锁定"
          note="系统锁屏、休眠或监测中断时锁定，默认开启。"
        >
          <Switch
            aria-label="系统事件锁定"
            checked={system}
            onCheckedChange={setSystem}
          />
        </SettingRow>
        <p className="settings-footnote">{state.system.message}</p>
        <p className="field-note">
          通知处理存在延迟；进程停顿超过 15 秒也会锁定。不承诺彻底擦除内存。
        </p>
        <Button type="submit" className="section-action">
          {task.busy ? "正在保存…" : "保存锁定设置"}
        </Button>
      </fieldset>
      <ErrorNotice message={task.error} />
    </form>
  );
}
function PasswordDialog({
  kind,
  format,
  onClose,
}: {
  kind: "password" | "upgrade";
  format: number;
  onClose: () => void;
}) {
  const [current, setCurrent] = useState(""),
    [next, setNext] = useState(""),
    [repeat, setRepeat] = useState(""),
    [confirmed, setConfirmed] = useState(false),
    task = useTask(),
    confirm = useConfirm();
  const upgrade = kind === "upgrade";
  return (
    <Modal
      title={upgrade ? "升级资料库格式" : "变更主密码"}
      busy={task.busy}
      onClose={onClose}
      footer={
        <>
          <Button variant="outline" disabled={task.busy} onClick={onClose}>
            取消
          </Button>
          <Button type="submit" form="password-change" disabled={task.busy}>
            {task.busy
              ? "正在处理…"
              : upgrade
                ? "升级为格式 3 并锁定"
                : "变更密码并锁定"}
          </Button>
        </>
      }
    >
      <Notice>
        {upgrade
          ? `当前为格式 ${format}，原有内容仍可读写。升级不改写历史；完成后锁定、停用设备授权与浏览器记忆，主密码不变。`
          : "先备份并暂停其他设备写入。变更只重新保护原库密钥，旧备份仍用旧密码，不能撤销已复制的库密钥。完成后锁定并停用设备授权。"}
      </Notice>
      <form
        id="password-change"
        onSubmit={(e) => {
          e.preventDefault();
          void task.run(async (ctx) => {
            try {
              if (upgrade && !confirmed)
                throw new Error("请先完成备份并更新其他设备程序");
              if (!upgrade) {
                validatePassword(next, repeat);
                if (current === next) throw new Error("新主密码不能与当前相同");
                if (
                  !(await confirm({
                    title: "确认变更主密码？",
                    description:
                      "资料库将锁定，旧审批失效。本机自动解锁会关闭，其他设备同步新配置后需重新授权。",
                  }))
                )
                  return;
              }
              const result = await ctx.request<{ warning?: string }>(
                upgrade ? "/api/vault/upgrade" : "/api/vault/password",
                upgrade
                  ? { password: current, confirmed: true, target_format: 3 }
                  : { current_password: current, new_password: next },
              );
              await ctx.refresh(
                (upgrade
                  ? "已升级为格式 3，请使用原主密码解锁。"
                  : "主密码已变更，请使用新密码解锁。") +
                  (result.warning || ""),
              );
            } finally {
              setCurrent("");
              setNext("");
              setRepeat("");
            }
          });
        }}
      >
        <fieldset disabled={task.busy} className="field-grid">
          <Field id="current-password" label="当前主密码">
            <Input
              id="current-password"
              type="password"
              autoComplete="current-password"
              required
              value={current}
              onChange={(e) => setCurrent(e.target.value)}
            />
          </Field>
          {!upgrade && (
            <>
              <Field
                id="new-password"
                label="新主密码"
                note="至少 12 个字符，最多 1024 字节"
              >
                <Input
                  id="new-password"
                  type="password"
                  autoComplete="new-password"
                  required
                  value={next}
                  onChange={(e) => setNext(e.target.value)}
                />
              </Field>
              <Field id="repeat-password" label="再次输入新主密码">
                <Input
                  id="repeat-password"
                  type="password"
                  autoComplete="new-password"
                  required
                  value={repeat}
                  onChange={(e) => setRepeat(e.target.value)}
                />
              </Field>
            </>
          )}
          {upgrade && (
            <label className="check-row">
              <input
                type="checkbox"
                checked={confirmed}
                onChange={(e) => setConfirmed(e.target.checked)}
                required
              />
              <span>
                我已备份、更新所有设备程序并暂停其他设备写入，确认升级为格式
                3。旧程序不能打开升级后的资料库。
              </span>
            </label>
          )}
        </fieldset>
        <ErrorNotice message={task.error} />
      </form>
    </Modal>
  );
}
