import { Inbox } from "lucide-react";
import { Button } from "@/components/ui/button";
import { EmptyState, IconButton } from "./shared";

export function ApprovalNotice({
  pending,
  onOpen,
}: {
  pending: boolean;
  onOpen: () => void;
}) {
  if (!pending) return null;
  return (
    <IconButton
      className="approval-notice"
      label="1 个待处理授权"
      onClick={onOpen}
    >
      <Inbox />
      <span className="request-count" aria-hidden="true">
        1
      </span>
    </IconButton>
  );
}

export function ApprovalView({
  pending,
  onDecision,
  onBack,
}: {
  pending: boolean;
  onDecision: (allowed: boolean) => void;
  onBack: () => void;
}) {
  return (
    <section className="approvals-view scroll-region">
      <div className="page-heading">
        <h1>授权请求</h1>
        <p className="field-note">通常在 AI 客户端确认，此处可备用处理。</p>
      </div>
      {pending ? (
        <article className="approval-item">
          <header className="approval-header">
            <Inbox />
            <div>
              <h2>读取资料</h2>
              <p>示例客户端</p>
            </div>
            <span className="preview-badge">示例请求</span>
          </header>
          <div className="approval-details">
            <dl>
              <dt>条目</dt>
              <dd>项目/发布清单</dd>
              <dt>请求范围</dt>
              <dd>正文，不包含附件</dd>
              <dt>执行方式</dt>
              <dd>仅本次，不改变长期权限</dd>
            </dl>
            <p className="field-note">
              同意读取后，获准的内容会提供给请求客户端。
            </p>
          </div>
          <footer className="approval-actions">
            <Button variant="outline" onClick={() => onDecision(false)}>
              拒绝
            </Button>
            <Button onClick={() => onDecision(true)}>同意本次读取</Button>
          </footer>
        </article>
      ) : (
        <EmptyState
          icon={<Inbox />}
          title="没有待处理的授权请求"
          action={
            <Button variant="outline" onClick={onBack}>
              返回资料库
            </Button>
          }
        />
      )}
    </section>
  );
}
