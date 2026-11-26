import {
  ArchiveRestore,
  ChevronDown,
  FolderClosed,
  FolderOpen,
  Layers3,
  LockKeyhole,
  Plus,
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
import { Brand } from "./shared";
import { groupsOf, type DialogKind, type Entry, type View } from "./model";
import { cn } from "@/lib/utils";

export function VaultMenu({ onOpen }: { onOpen: (kind: DialogKind) => void }) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button className="vault-switch">
          <FolderClosed />
          <span>个人资料</span>
          <ChevronDown />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-52">
        <DropdownMenuItem onSelect={() => onOpen("open-vault")}>
          <FolderOpen />
          打开其他资料库
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => onOpen("create-vault")}>
          <Plus />
          创建资料库
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={() => onOpen("restore-vault")}>
          <ArchiveRestore />
          从备份恢复
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
export function Sidebar({
  entries,
  view,
  group,
  onGo,
  onLock,
  onOpen,
}: {
  entries: Entry[];
  view: View;
  group: string | null;
  onGo: (view: View, group?: string | null) => void;
  onLock: () => void;
  onOpen: (kind: DialogKind) => void;
}) {
  const live = entries.filter((entry) => !entry.deleted);
  return (
    <aside className="sidebar">
      <div className="sidebar-brand">
        <Brand />
      </div>
      <VaultMenu onOpen={onOpen} />
      <nav className="primary-nav" aria-label="主导航">
        <button
          className={cn(
            "nav-item",
            view === "entries" && group === null && "active",
          )}
          aria-current={
            view === "entries" && group === null ? "page" : undefined
          }
          onClick={() => onGo("entries")}
        >
          <Layers3 />
          <span>全部资料</span>
          <span className="count">{live.length}</span>
        </button>
      </nav>
      <div className="group-section scroll-region">
        <h2 className="nav-label">分组</h2>
        <nav className="group-nav" aria-label="资料分组">
          {[...groupsOf(entries), ""].map((name) => (
            <button
              key={name}
              className={cn(
                "nav-item",
                view === "entries" && group === name && "active",
              )}
              aria-current={
                view === "entries" && group === name ? "page" : undefined
              }
              onClick={() => onGo("entries", name)}
            >
              <FolderClosed />
              <span>{name || "未分组"}</span>
              <span className="count">
                {live.filter((entry) => entry.group === name).length}
              </span>
            </button>
          ))}
        </nav>
      </div>
      <nav className="sidebar-footer" aria-label="管理">
        <button
          className={cn("nav-item", view === "trash" && "active")}
          aria-current={view === "trash" ? "page" : undefined}
          onClick={() => onGo("trash")}
        >
          <Trash2 />
          <span>回收站</span>
          <span className="count">{entries.length - live.length}</span>
        </button>
        <button
          className={cn("nav-item", view === "settings" && "active")}
          aria-current={view === "settings" ? "page" : undefined}
          onClick={() => onGo("settings")}
        >
          <Settings2 />
          <span>设置</span>
        </button>
        <button className="nav-item" onClick={onLock}>
          <LockKeyhole />
          <span>锁定资料库</span>
        </button>
      </nav>
    </aside>
  );
}
