import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field } from "@/components/common";
import { ErrorNotice, Notice, useConfirm } from "@/components/forms";
import { useClient, useResource, useTask } from "@/lib/hooks";
import { errorText, StaleRequest } from "@/lib/client";
import type { Client } from "@/lib/types";
type MCPConfig = { mode: "stdio" | "http"; command: string; args: string[]; url: string };
export function MCPSettings() {
  const resource = useResource<MCPConfig>("/api/mcp/settings");
  const task = useTask();
  const confirm = useConfirm();
  const client = useClient();
  const config = resource.data;
  const change = (mode: "stdio" | "http") => task.run(async (ctx) => {
    if (!(await confirm({
      title: mode === "http" ? "启用 HTTP MCP？" : "切回 stdio？",
      description: "切换后将锁定资料库，需重新解锁并重新连接 AI 客户端。",
    }))) return;
    await ctx.request("/api/mcp/settings", { mode, confirmed: true });
    await ctx.refresh("MCP 接入方式已更新，请重新解锁。");
  });
  return (
    <>
      <section className="settings-section">
        <h2>MCP 接入方式</h2>
        <p className="settings-footnote">选择 AI 客户端的接入方式。推荐默认 stdio，无需手动管理凭据。</p>
        <ErrorNotice message={resource.error || task.error} />
        {!config ? <p>正在读取接入设置…</p> : <>
          <div className="inline-actions">
            <Button variant={config.mode === "stdio" ? "default" : "outline"} disabled={task.busy || config.mode === "stdio"} onClick={() => void change("stdio")}>stdio（推荐）</Button>
            <Button variant={config.mode === "http" ? "default" : "outline"} disabled={task.busy || config.mode === "http"} onClick={() => void change("http")}>HTTP（高级）</Button>
          </div>
          {config.mode === "stdio" && <>
            <p className="settings-footnote">保持资料匣运行并解锁，然后将下方配置添加到 AI 客户端：</p>
            <pre className="path-value">{JSON.stringify({ mcpServers: { foliochest: { command: config.command, args: config.args } } }, null, 2)}</pre>
            <Button variant="outline" disabled={task.busy} onClick={() => void task.run(async (ctx) => {
              ctx.assert();
              await navigator.clipboard.writeText(JSON.stringify({ mcpServers: { foliochest: { command: config.command, args: config.args } } }, null, 2));
            }, () => client.notice("配置已复制。"))}>复制 stdio 配置</Button>
          </>}
        </>}
      </section>
      {config?.mode === "http" && <HTTPMCPSettings />}
    </>
  );
}
function HTTPMCPSettings() {
  const [clients, setClients] = useState<Client[]>([]),
    [name, setName] = useState(""),
    [token, setToken] = useState(""),
    [loadError, setLoadError] = useState(""),
    task = useTask(),
    client = useClient(),
    confirm = useConfirm();
  useEffect(() => {
    let active = true;
    client
      .request<Client[]>("/api/clients")
      .then((data) => {
        if (active) setClients(data || []);
      })
      .catch((e) => {
        if (active && !(e instanceof StaleRequest)) setLoadError(errorText(e));
      });
    return () => {
      active = false;
    };
  }, [client]);
  const revoke = (item: Client) =>
    task.run(
      async (ctx) => {
        if (
          !(await confirm({
            title: "撤销接入凭据？",
            description: `「${item.name}」的新请求和未完成审批将被拒绝。`,
            label: "撤销",
            danger: true,
          }))
        )
          return false;
        await ctx.request("/api/clients/revoke", { id: item.id });
        return true;
      },
      (ok) => {
        if (ok) {
          setClients((before) => before.filter((c) => c.id !== item.id));
          setToken("");
          client.notice("接入凭据已撤销。");
        }
      },
    );
  return (
    <section className="settings-section">
      <h2>本机 MCP</h2>
      <p className="path-value">{location.origin}/mcp</p>
      <p className="settings-footnote">
        为每个 AI 客户端创建独立凭据，访问时仍受单条资料权限控制。
      </p>
      <form
        className="section-action"
        onSubmit={(e) => {
          e.preventDefault();
          void task.run(
            (ctx) =>
              ctx.request<{ client: Client; token: string; url: string }>(
                "/api/clients/create",
                { name },
              ),
            (result) => {
              setToken(result.token);
              setClients((before) => [...before, result.client]);
              setName("");
            },
          );
        }}
      >
        <fieldset className="client-create" disabled={task.busy}>
          <Field id="client-name" label="客户端名称">
            <Input
              id="client-name"
              value={name}
              maxLength={120}
              required
              onChange={(e) => setName(e.target.value)}
              placeholder="例如：本机 pi"
            />
          </Field>
          <Button type="submit">创建接入凭据</Button>
        </fieldset>
      </form>
      {token && (
        <div className="secret-box">
          <Notice>
            接入凭据仅显示一次，请妥善保存到客户端配置中。
          </Notice>
          <pre className="secret-token">{token}</pre>
          <p className="field-note">Authorization: Bearer &lt;接入凭据&gt;</p>
          <div className="inline-actions">
            <Button
              variant="outline"
              disabled={task.busy}
              onClick={() =>
                void task.run(
                  async (ctx) => {
                    ctx.assert();
                    await navigator.clipboard.writeText(token);
                  },
                  () => client.notice("凭据已复制，请妥善保存。"),
                )
              }
            >
              复制凭据
            </Button>
            <Button variant="ghost" onClick={() => setToken("")}>
              已保存，隐藏
            </Button>
          </div>
        </div>
      )}
      <ErrorNotice message={loadError || task.error} />
      <div className="client-list">
        {clients.map((item) => (
          <div className="setting-row" key={item.id}>
            <div>
              <h3>{item.name}</h3>
              <p>创建于 {new Date(item.created_at).toLocaleString()}</p>
            </div>
            <Button
              variant="outline"
              disabled={task.busy}
              onClick={() => void revoke(item)}
            >
              撤销
            </Button>
          </div>
        ))}
      </div>
      {!clients.length && !loadError && (
        <p className="settings-footnote">还没有接入的客户端。</p>
      )}
    </section>
  );
}
