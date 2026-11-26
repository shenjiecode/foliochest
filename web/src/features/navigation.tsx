import type { ReactNode } from "react";
import {
  ArchiveRestore,
  ChevronDown,
  FolderClosed,
  FolderOpen,
  Layers3,
  LockKeyhole,
  Plus,
  Power,
  Settings2,
  Trash2,
} from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Brand } from "@/components/common";
import { cn } from "@/lib/utils";
import { type Group, type VaultAction, vaultName } from "@/lib/types";
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
  onGo,
  onLock,
  onQuit,
  onOpen,
}: {
  directory: string;
  count: number;
  groups: Group[];
  view: View;
  children?: ReactNode;
  onGo: (view: View) => void;
  onLock: () => void;
  onQuit: () => void;
  onOpen: (kind: VaultAction) => void;
}) {
  return (
    <aside className="sidebar">
      <div className="sidebar-brand">
        <Brand />
      </div>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button className="vault-switch" title={directory}>
            <FolderClosed />
            <span>{vaultName(directory)}</span>
            <ChevronDown />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          <DropdownMenuItem onSelect={() => onOpen("open")}>
            <FolderOpen />
            打开其他资料库
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => onOpen("create")}>
            <Plus />
            创建资料库
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem onSelect={() => onOpen("restore")}>
            <ArchiveRestore />
            从备份恢复
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
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
          <span className="count">
            {groups.reduce((n, g) => n + g.deleted_count, 0)}
          </span>
        </button>
        <button
          className={cn("nav-item", view === "settings" && "active")}
          onClick={() => onGo("settings")}
        >
          <Settings2 />
          <span>设置</span>
        </button>
        <button className="nav-item" onClick={onLock}>
          <LockKeyhole />
          <span>锁定资料库</span>
        </button>
        <button className="nav-item" onClick={onQuit}>
          <Power />
          <span>退出资料匣</span>
        </button>
      </nav>
    </aside>
  );
}
