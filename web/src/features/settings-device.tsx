import { useState } from "react";
import { Power, LockKeyhole, PanelTop } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Field, SettingRow } from "@/components/common";
import { ErrorNotice, Notice } from "@/components/forms";
import { useClient, useResource, useTask, type TaskContext } from "@/lib/hooks";
import type { Device } from "@/lib/types";
export function DeviceSettings() {
  const resource = useResource<Device>("/api/device");
  return (
    <section className="settings-section device-settings">
      <ErrorNotice message={resource.error} />
      {resource.loading ? (
        <p>正在读取设备状态…</p>
      ) : (
        resource.data && <DeviceForm initial={resource.data} />
      )}
    </section>
  );
}
function DeviceForm({ initial }: { initial: Device }) {
  const [device, setDevice] = useState(initial),
    [startup, setStartup] = useState(initial.startup.enabled),
    [auto, setAuto] = useState(initial.auto_unlock),
    [remember, setRemember] = useState(initial.remembered_browser),
    [password, setPassword] = useState(""),
    task = useTask(),
    client = useClient();
  const readBack = async (ctx: TaskContext) => {
    const actual = await ctx.request<Device>("/api/device");
    if (ctx.current()) {
      setDevice(actual);
      setStartup(actual.startup.enabled);
      setAuto(actual.auto_unlock);
      setRemember(actual.remembered_browser);
    }
  };
  const change = async (ctx: TaskContext, path: string, body: unknown) => {
    let result: { warning?: string };
    try {
      result = await ctx.request(path, body);
    } catch (error) {
      await readBack(ctx).catch(() => {});
      throw error;
    }
    await readBack(ctx);
    return result;
  };
  const save = (disable = false) =>
    task.run(
      async (ctx) => {
        try {
          return await change(ctx, "/api/device/unlock", {
            enabled: disable ? false : auto,
            ...(!disable && auto
              ? { password, remember_browser: remember }
              : {}),
          });
        } finally {
          setPassword("");
        }
      },
      (result) =>
        client.notice(
          result.warning || "设备授权已保存。",
        ),
    );
  return (
    <>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void task.run(
            (ctx) => change(ctx, "/api/device/startup", { enabled: startup }),
            () => client.notice("启动设置已保存。"),
          );
        }}
      >
        <fieldset disabled={task.busy}>
          <SettingRow
            title="登录系统后启动"
            icon={<Power />}
            status={
              startup !== device.startup.enabled
                ? "未保存"
                : startup
                  ? "已启用"
                  : "已禁用"
            }
            note="开机登录后在后台自动运行。移动程序位置后需重新保存。"
          >
            <Switch
              aria-label="登录系统后启动"
              checked={startup}
              disabled={!device.startup.supported}
              onCheckedChange={setStartup}
            />
          </SettingRow>
          {device.startup_error && <Notice>{device.startup_error}</Notice>}
          {device.startup.enabled && !device.startup.current && (
            <Notice>启动项与当前程序位置不一致，请重新保存。</Notice>
          )}
          <Button
            className="section-action"
            variant="outline"
            disabled={!device.startup.supported}
            type="submit"
          >
            保存启动设置
          </Button>
        </fieldset>
      </form>
      <form
        className="settings-section"
        onSubmit={(e) => {
          e.preventDefault();
          void save();
        }}
      >
        <fieldset disabled={task.busy}>
          <SettingRow
            title="服务启动时自动解锁"
            icon={<LockKeyhole />}
            status={
              auto !== device.auto_unlock
                ? "未保存"
                : auto
                  ? "已启用"
                  : "已禁用"
            }
            note={`使用系统凭据库（${device.provider}）自动解锁。`}
          >
            <Switch
              aria-label="自动解锁"
              checked={auto}
              onCheckedChange={(value) => {
                setAuto(value);
                if (!value) setPassword("");
              }}
            />
          </SettingRow>
          {device.auto_unlock && !device.matches_vault && (
            <Notice>
              已有授权与当前资料库不一致，请重新授权或关闭并清理。
            </Notice>
          )}
          <SettingRow
            title="记住这个浏览器"
            icon={<PanelTop />}
            status={
              remember !== device.remembered_browser
                ? "未保存"
                : remember
                  ? "已启用"
                  : "已禁用"
            }
            note="免重复登录，有效期 90 天。"
          >
            <Switch
              aria-label="记住这个浏览器"
              checked={remember}
              disabled={!auto}
              onCheckedChange={setRemember}
            />
          </SettingRow>
          {auto && (
            <div className="field-grid section-action">
              <Notice>
                授权保存在本机系统凭据库中，不保存主密码。
              </Notice>

              <Field id="device-password" label="再次输入主密码">
                <Input
                  id="device-password"
                  type="password"
                  autoComplete="current-password"
                  required
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                />
              </Field>
            </div>
          )}
          <Button className="section-action" variant="outline" type="submit">
            保存设备授权
          </Button>
          {device.cleanup_pending && (
            <Button
              className="section-action"
              variant="outline"
              type="button"
              onClick={() => void save(true)}
            >
              重试清理旧凭据
            </Button>
          )}
        </fieldset>
      </form>
      <ErrorNotice message={task.error} />
      {device.notice && <Notice>{device.notice}</Notice>}
      <p className="settings-footnote">以上设置仅对当前电脑生效。</p>
    </>
  );
}
