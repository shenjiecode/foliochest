import { useState } from "react";
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
    <section className="settings-section">
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
          result.warning ||
            "设备授权已保存。关闭授权不结束当前会话，需要时另行锁定。",
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
            note="不打开浏览器，不代表自动解锁。程序应放在固定位置，移动后重新登记。"
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
            note={device.provider + "；默认关闭，每次进程启动只尝试一次。"}
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
          {auto && (
            <div className="field-grid section-action">
              <Notice>
                系统凭据保存库密钥，不保存主密码。同一系统账户下的恶意程序可能读取凭据。系统凭据不可用时仍需主密码。
              </Notice>
              <SettingRow
                title="记住这个浏览器"
                note="有效期 90 天，新授权替换旧记忆；不认证其他浏览器。"
              >
                <Switch
                  aria-label="记住这个浏览器"
                  checked={remember}
                  onCheckedChange={setRemember}
                />
              </SettingRow>
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
      <p className="settings-footnote">
        以上设置只影响这台电脑，不随资料库同步。
      </p>
    </>
  );
}
