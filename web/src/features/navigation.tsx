import type { ReactNode } from "react";
import {
  FolderClosed,
  Layers3,
  LockKeyhole,
  Plus,
  Power,
  Settings2,
  Trash2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Brand } from "@/components/common";
import { cn } from "@/lib/utils";
import { type Group } from "@/lib/types";
export type View = "entries" | "trash" | "settings" | "approvals";
export const titles: Record<View, string> = {
  entries: "资料库",
  trash: "回收站",
  settings: "设置",
  approvals: "授权请求",
};
export function Sidebar({
  directory,
  count,
  groups,
  view,
  children,
  notice,
  onGo,
  onLock,
  onQuit,
  onNew,
}: {
  directory: string;
  count: number;
  groups: Group[];
  view: View;
  children?: ReactNode;
  notice?: ReactNode;
  onGo: (view: View) => void;
  onLock: () => void;
  onQuit: () => void;
  onNew: () => void;
}) {
  return (
    <aside className="sidebar">
      <div className="sidebar-brand">
        <Brand />
        {notice}
      </div>
      <div className="vault-picker-row">
        <div className="vault-switch single-vault-label" title={directory}>
          <FolderClosed />
          <span>全部资料</span>
          <span className="count">{count}</span>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={onNew}
          aria-label="新建资料"
        >
          <Plus />
          新建
        </Button>
      </div>
      {view !== "entries" && (
        <nav className="primary-nav" aria-label="主导航">
          <button className="nav-item" onClick={() => onGo("entries")}>
            <Layers3 />
            <span>全部资料</span>
            <span className="count">{count}</span>
          </button>
        </nav>
      )}
      {children || <div className="sidebar-spacer" />}
      <nav className="sidebar-footer" aria-label="管理">
        <button
          className={cn("nav-item", view === "trash" && "active")}
          onClick={() => onGo("trash")}
        >
          <Trash2 />
          <span>回收站</span>
          {groups.some((g) => g.deleted_count > 0) && (
            <span className="count">
              {groups.reduce((n, g) => n + g.deleted_count, 0)}
            </span>
          )}
        </button>
        <button
          className={cn("nav-item", view === "settings" && "active")}
          onClick={() => onGo("settings")}
        >
          <Settings2 />
          <span>设置</span>
        </button>
        <div className="sidebar-session-actions">
          <button className="nav-item" aria-label="锁定资料库" onClick={onLock}>
            <LockKeyhole />
            <span>锁定</span>
          </button>
          <button className="nav-item" aria-label="退出资料匣" onClick={onQuit}>
            <Power />
            <span>退出</span>
          </button>
        </div>
      </nav>
    </aside>
  );
}
