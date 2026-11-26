import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field } from "@/components/common";
import { ErrorNotice, Notice, useConfirm } from "@/components/forms";
import { useClient, useTask } from "@/lib/hooks";
import { errorText, StaleRequest } from "@/lib/client";
import type { Client } from "@/lib/types";
export function MCPSettings() {
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
        每个 harness 使用独立凭据；主密码不提供给
        AI。支持交互确认的客户端在原处审批，否则使用网页的授权提示。
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
            接入凭据仅显示一次。请保存到 harness
            配置，不要发到聊天或提交到仓库。
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
