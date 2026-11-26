import { useEffect, useState } from "react";
import { Check, ChevronRight, FileText, Menu, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Sidebar } from "./navigation";
import { ApprovalNotice, ApprovalView } from "./approval";
import { EntryList } from "./entry-list";
import { EntryView } from "./entry-view";
import { Unlock } from "./unlock";
import { SettingsView } from "./settings";
import { Dialogs } from "./dialogs";
import { EmptyState, IconButton, PreviewBadge } from "./shared";
import {
  exampleEntries,
  previewOptions,
  saveExample,
  visibleEntries,
  type DialogKind,
  type Entry,
  type View,
} from "./model";
const titles: Record<View, string> = {
  entries: "资料库",
  approvals: "授权请求",
  trash: "回收站",
  settings: "设置",
};
export function App() {
  const [entries, setEntries] = useState(exampleEntries);
  const [unlocked, setUnlocked] = useState(
    () => previewOptions(location.search).unlocked,
  );
  const [view, setView] = useState<View>("entries");
  const [group, setGroup] = useState<string | null>(null);
  const [selected, setSelected] = useState<string | null>("release");
  const [query, setQuery] = useState("");
  const [bodySearch, setBodySearch] = useState(false);
  const [checked, setChecked] = useState<Set<string>>(new Set());
  const [dialog, setDialog] = useState<DialogKind>(null);
  const [mobileNav, setMobileNav] = useState(false);
  const [detailOpen, setDetailOpen] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [editorEpoch, setEditorEpoch] = useState(0);
  const [pendingAction, setPendingAction] = useState<(() => void) | null>(null);
  const [pendingRequest, setPendingRequest] = useState(
    () => previewOptions(location.search).pendingRequest,
  );
  const [toast, setToast] = useState("");
  useEffect(() => {
    if (!toast) return;
    const id = window.setTimeout(() => setToast(""), 5000);
    return () => clearTimeout(id);
  }, [toast]);
  useEffect(() => {
    if (!dirty) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);
  const guard = (action: () => void) => {
    if (dirty) setPendingAction(() => action);
    else action();
  };
  const go = (next: View, nextGroup: string | null = null) =>
    guard(() => {
      setView(next);
      setGroup(nextGroup);
      setQuery("");
      setChecked(new Set());
      setMobileNav(false);
      setDetailOpen(false);
      setSelected(
        next === "entries"
          ? (entries.find(
              (entry) =>
                !entry.deleted &&
                (nextGroup === null || entry.group === nextGroup),
            )?.id ?? null)
          : null,
      );
    });
  const open = (kind: DialogKind) =>
    guard(() => {
      setDialog(kind);
      setMobileNav(false);
    });
  const save = (next: Entry) => {
    setEntries((before) => saveExample(before, next));
    setDirty(false);
    if (view === "trash" && !next.deleted) {
      setView("entries");
      setGroup(null);
    }
    setToast("已更新示例资料；刷新页面后重置。");
  };
  const lock = () =>
    guard(() => {
      setUnlocked(false);
      setEntries(exampleEntries());
      setDialog(null);
      setSelected("release");
      setDirty(false);
      setChecked(new Set());
      setQuery("");
      setGroup(null);
      setView("entries");
      setMobileNav(false);
      setToast("");
      setPendingRequest(false);
      setDetailOpen(false);
    });
  const current = entries.find((entry) => entry.id === selected);
  const shown = visibleEntries(
    entries,
    group,
    query,
    bodySearch,
    view === "trash",
  );
  const sidebar = (
    <Sidebar
      entries={entries}
      view={view}
      group={group}
      onGo={go}
      onLock={lock}
      onOpen={open}
    />
  );
  return (
    <>
      {!unlocked ? (
        <Unlock onUnlock={() => setUnlocked(true)} onOpen={setDialog} />
      ) : (
        <div className="app-layout">
          <div className="desktop-sidebar">{sidebar}</div>
          <main className="main-shell">
            <header className="topbar">
              <IconButton
                className="mobile-menu"
                label="打开导航"
                onClick={() => setMobileNav(true)}
              >
                <Menu />
              </IconButton>
              <div className="breadcrumbs">
                <span className="breadcrumb-vault">个人资料</span>
                <ChevronRight />
                <strong>{titles[view]}</strong>
              </div>
              <ApprovalNotice
                pending={pendingRequest}
                onOpen={() => go("approvals")}
              />
              <PreviewBadge />
            </header>
            {(view === "entries" || view === "trash") && (
              <div className="workspace" data-detail-open={detailOpen}>
                <EntryList
                  entries={shown}
                  group={group}
                  query={query}
                  bodySearch={bodySearch}
                  trash={view === "trash"}
                  selected={selected}
                  checked={checked}
                  onQuery={setQuery}
                  onBodySearch={setBodySearch}
                  onSelect={(id) =>
                    guard(() => {
                      setSelected(id);
                      setDetailOpen(true);
                    })
                  }
                  onCheck={(id, value) =>
                    setChecked((before) => {
                      const next = new Set(before);
                      if (value) next.add(id);
                      else next.delete(id);
                      return next;
                    })
                  }
                  onNew={() => open("new")}
                  onMove={() => open("move")}
                  onClear={() => setChecked(new Set())}
                />
                {current ? (
                  <EntryView
                    key={current.id + "-" + editorEpoch}
                    entry={current}
                    groups={[
                      ...new Set(
                        entries.map((entry) => entry.group).filter(Boolean),
                      ),
                    ]}
                    onSave={save}
                    onDirty={setDirty}
                    onOpen={open}
                    onBack={() => guard(() => setDetailOpen(false))}
                  />
                ) : (
                  <div className="entry-panel">
                    <EmptyState
                      icon={<FileText />}
                      title={
                        view === "trash"
                          ? "选择一条已删除的资料"
                          : "选择一条资料"
                      }
                    >
                      {view === "trash"
                        ? "从删除前的历史版本恢复。"
                        : undefined}
                    </EmptyState>
                  </div>
                )}
              </div>
            )}
            {view === "settings" && (
              <SettingsView onOpen={open} notice={setToast} />
            )}
            {view === "approvals" && (
              <ApprovalView
                pending={pendingRequest}
                onBack={() => go("entries")}
                onDecision={(allowed) => {
                  setPendingRequest(false);
                  setToast(
                    allowed
                      ? "已处理示例请求，未向任何客户端发送资料。"
                      : "已拒绝示例请求。",
                  );
                }}
              />
            )}
          </main>
        </div>
      )}
      <Dialog open={mobileNav} onOpenChange={setMobileNav}>
        <DialogContent
          className="navigation-drawer"
          onCloseAutoFocus={() =>
            document.querySelector<HTMLButtonElement>(".mobile-menu")?.focus()
          }
        >
          <DialogTitle className="sr-only">导航</DialogTitle>
          <DialogDescription className="sr-only">
            选择资料分组或管理页面
          </DialogDescription>
          {sidebar}
        </DialogContent>
      </Dialog>
      <Dialogs
        key={dialog || "closed"}
        kind={dialog}
        entries={entries}
        current={current}
        checked={checked}
        onClose={() => setDialog(null)}
        onEnter={() => {
          setDialog(null);
          setUnlocked(true);
        }}
        notice={setToast}
        onSave={save}
        onCreate={(entry) => {
          setEntries((before) => [...before, entry]);
          setView("entries");
          setGroup(null);
          setQuery("");
          setSelected(entry.id);
          setDetailOpen(true);
          setDialog(null);
          setToast("已创建示例条目。");
        }}
        onMove={(name) => {
          setEntries((before) =>
            before.map((entry) =>
              checked.has(entry.id) ? { ...entry, group: name } : entry,
            ),
          );
          setChecked(new Set());
          setGroup(null);
          setDialog(null);
          setToast("已移动选中的示例资料。");
        }}
        onDelete={() => {
          if (current)
            setEntries((before) =>
              before.map((entry) =>
                entry.id === current.id
                  ? { ...entry, value: "", attachments: [], deleted: true }
                  : entry,
              ),
            );
          setSelected(null);
          setDetailOpen(false);
          setDialog(null);
          setToast("示例条目已移到回收站。");
        }}
      />
      <Dialog
        open={!!pendingAction}
        onOpenChange={(value) => {
          if (!value) setPendingAction(null);
        }}
      >
        <DialogContent className="dialog-card">
          <DialogHeader>
            <DialogTitle>放弃未保存的修改？</DialogTitle>
            <DialogDescription>当前编辑内容尚未保存。</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPendingAction(null)}>
              继续编辑
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                const action = pendingAction;
                setPendingAction(null);
                setDirty(false);
                setEditorEpoch((value) => value + 1);
                action?.();
              }}
            >
              放弃修改
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      {toast && (
        <div className="toast" role="status">
          <Check />
          <span>{toast}</span>
          <button aria-label="关闭通知" onClick={() => setToast("")}>
            <X />
          </button>
        </div>
      )}
    </>
  );
}
