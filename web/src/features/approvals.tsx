import { Bell, ShieldCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import { EmptyState, IconButton } from "@/components/common";
import { ErrorNotice, Notice } from "@/components/forms";
import { useClient, useTask } from "@/lib/hooks";
import type { Pending } from "@/lib/types";
import { fileSize } from "@/lib/files";
export function ApprovalNotice({
  count,
  stale = false,
  onOpen,
}: {
  count: number;
  stale?: boolean;
  onOpen: () => void;
}) {
  return count ? (
    <div className="approval-notice">
      <IconButton label={count + " 个待处理授权" + (stale ? "（状态待确认）" : "")} onClick={onOpen}>
        <Bell />
      </IconButton>
      <span className="request-count" aria-hidden="true">
        {count}
      </span>
    </div>
  ) : null;
}
export function Approvals({
  requests,
  loading = false,
  error = "",
  onRefresh,
  onBack,
}: {
  requests: Pending[];
  loading?: boolean;
  error?: string;
  onRefresh: () => void;
  onBack: () => void;
}) {
  return (
    <section className="approvals-view scroll-region" aria-label="授权请求">
      <header className="page-heading">
        <h1>授权请求</h1>
        <Button variant="outline" disabled={loading} onClick={onRefresh}>
          {loading ? "正在刷新审批…" : "刷新审批状态"}
        </Button>
      </header>
      <ErrorNotice message={error} />
      {error && !!requests.length && <Notice>暂时无法刷新，当前显示上次成功读取的列表。</Notice>}
      {!requests.length && !loading && !error && (
        <p className="field-note">授权请求在 5 分钟内有效，锁定资料库或客户端断开后会自动失效。</p>
      )}
      {!requests.length ? (
        <EmptyState
          icon={<ShieldCheck />}
          title={error ? "暂时无法确认待审批状态" : loading ? "正在读取授权请求" : "没有待处理的授权请求"}
          action={
            <Button variant="outline" onClick={onBack}>
              返回资料库
            </Button>
          }
        />
      ) : (
        requests.map((request) => (
          <Approval key={request.id} request={request} onRefresh={onRefresh} />
        ))
      )}
    </section>
  );
}
function Approval({
  request,
  onRefresh,
}: {
  request: Pending;
  onRefresh: () => void;
}) {
  const task = useTask(),
    client = useClient();
  const decide = (accept: boolean) =>
    task.run(
      async (ctx) => {
        try {
          await ctx.request("/api/approvals/decide", {
            id: request.id,
            accept,
          });
        } finally {
          if (ctx.current()) onRefresh();
        }
      },
      () => client.notice(accept ? "本次请求已处理。" : "已拒绝本次请求。"),
    );
  return (
    <article className="approval-item">
      <header className="approval-header">
        <ShieldCheck />
        <div>
          <h2>{request.client_name}</h2>
          <p>
            {request.operation === "read" ? "读取请求" : "替换请求"} · 有效至{" "}
            {new Date(request.expires_at).toLocaleTimeString()}
          </p>
        </div>
      </header>
      <div className="approval-details">
        {request.reads?.map((read, i) => (
          <dl key={i}>
            <dt>Key</dt>
            <dd>{read.key}</dd>
            <dt>读取范围</dt>
            <dd>
              {read.include_value ? "正文；" : ""}
              {read.all_attachments
                ? "全部附件"
                : read.attachments?.join("、") || "无附件"}
            </dd>
            {read.revision && (
              <>
                <dt>版本</dt>
                <dd className="path-value">{read.revision}</dd>
              </>
            )}
          </dl>
        ))}
        {request.changes?.map((change, i) => (
          <section key={i}>
            <h3>{change.key}</h3>
            <p className="field-note">预期版本 {change.expected_revision}</p>
            {change.description !== undefined && (
              <p>描述：{change.description}</p>
            )}
            {change.format !== undefined && <p>格式：{change.format}</p>}
            {change.value !== undefined && (
              <details>
                <summary>将要替换的正文</summary>
                <pre className="approval-content">{change.value}</pre>
              </details>
            )}
            {change.attachments !== undefined && (
              <div>
                替换附件列表：
                {change.attachments.length
                  ? change.attachments.map((a) => (
                      <p key={a.name}>
                        {a.name} · {Math.ceil(fileSize(a.data) / 1024)} KiB
                      </p>
                    ))
                  : "清空附件"}
              </div>
            )}
          </section>
        ))}
        <Notice>
          {request.operation === "read"
            ? "同意后，该 AI 客户端将读取上述内容。"
            : "同意后，将按上述内容更新对应资料。"}
        </Notice>
        <ErrorNotice message={task.error} />
      </div>
      <footer className="approval-actions">
        <Button
          variant="outline"
          disabled={task.busy}
          onClick={() => void decide(false)}
        >
          拒绝
        </Button>
        <Button disabled={task.busy} onClick={() => void decide(true)}>
          {task.busy ? "正在处理…" : "同意本次操作"}
        </Button>
      </footer>
    </article>
  );
}
