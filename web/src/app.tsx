import { useEffect, useState } from "react";
import { FileText, Menu, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Brand, EmptyState, IconButton } from "@/components/common";
import { ConfirmProvider, ErrorNotice, useConfirm } from "@/components/forms";
import { SessionClient } from "@/lib/client";
import { ExitContext, useQuit } from "@/components/exit-app";
import {
  ClientContext,
  useClient,
  useDirty,
  useSession,
  useTask,
} from "@/lib/hooks";
import { useBodySearch, useCatalog } from "@/lib/catalog";
import { filterEntries } from "@/lib/groups";
import {
  type AppState,
  type Entry,
  type Group,
  type Member,
  type VaultAction,
} from "@/lib/types";
import { Unlock, VaultFlow } from "@/features/unlock";
import { Sidebar, type View } from "@/features/navigation";
import { EntryList } from "@/features/entry-list";
import { EntryEditor } from "@/features/entry-editor";
import { EntryManagement, GroupManagement } from "@/features/management";
import { HistoryDialog } from "@/features/history";
import { Settings } from "@/features/settings";
import { ApprovalNotice, Approvals } from "@/features/approvals";

export function App({ client }: { client: SessionClient }) {
  const [exited, setExited] = useState(false);
  return (
    <ClientContext value={client}>
      <ExitContext value={() => setExited(true)}>
        {exited ? (
          <main className="lock-screen">
            <header className="lock-topbar">
              <Brand />
            </header>
            <div className="lock-center">
              <h1>资料匣已退出</h1>
              <p>可以关闭此页面。下次点击应用图标重新启动。</p>
            </div>
          </main>
        ) : (
          <SessionGate />
        )}
      </ExitContext>
    </ClientContext>
  );
}
function SessionGate() {
  const client = useClient(),
    snapshot = useSession();
  useEffect(() => {
    let pending = false;
    const refresh = async () => {
      if (pending) return;
      pending = true;
      try {
        await client.refresh();
      } finally {
        pending = false;
      }
    };
    void refresh();
    const timer = setInterval(() => void refresh(), 3000);
    let last = 0;
    const activity = (e: Event) => {
      if (
        !e.isTrusted ||
        !client.getSnapshot().state?.authenticated ||
        Date.now() - last < 15000
      )
        return;
      last = Date.now();
      void client.request("/api/activity", {}).catch(() => {
        last = 0;
      });
    };
    for (const name of ["pointerdown", "keydown", "input", "wheel"])
      window.addEventListener(name, activity, { passive: true });
    return () => {
      clearInterval(timer);
      for (const name of ["pointerdown", "keydown", "input", "wheel"])
        window.removeEventListener(name, activity);
    };
  }, [client]);
  useEffect(() => {
    if (!snapshot.notice) return;
    const timer = setTimeout(() => client.notice(""), 6000);
    return () => clearTimeout(timer);
  }, [snapshot.notice, client]);
  return (
    <>
      <ConfirmProvider key={snapshot.epoch}>
        {snapshot.state ? (
          snapshot.state.authenticated ? (
            <Workspace
              state={snapshot.state}
              connectionError={snapshot.error}
            />
          ) : (
            <Unlock state={snapshot.state} />
          )
        ) : (
          <main className="lock-screen">
            <header className="lock-topbar">
              <Brand />
            </header>
            <div className="lock-center">
              <p>{snapshot.error ? "无法连接本机服务" : "正在连接本机服务…"}</p>
              <ErrorNotice message={snapshot.error} />
              {snapshot.error && (
                <Button onClick={() => void client.refresh()}>重试</Button>
              )}
            </div>
          </main>
        )}
      </ConfirmProvider>
      {snapshot.notice && (
        <div className="toast" role="status">
          <span>{snapshot.notice}</span>
          <button aria-label="关闭通知" onClick={() => client.notice("")}>
            <X />
          </button>
        </div>
      )}
      {snapshot.error && snapshot.state && !snapshot.state.authenticated && (
        <div className="connection-error" role="alert">
          {snapshot.error}
        </div>
      )}
    </>
  );
}
type DialogState =
  | { kind: "vault"; action: VaultAction }
  | { kind: "history"; key: string }
  | { kind: "rename" | "delete"; entry: Entry }
  | { kind: "group"; source?: Group }
  | null;
function Workspace({
  state,
  connectionError,
}: {
  state: AppState;
  connectionError: string;
}) {
  const client = useClient(),
    confirm = useConfirm(),
    task = useTask(),
    exit = useQuit();
  const [view, setView] = useState<View>(() =>
      location.hash === "#approvals" ? "approvals" : "entries",
    ),
    [selected, setSelected] = useState<string | null>(null),
    [isNew, setIsNew] = useState(false),
    [draftGroup, setDraftGroup] = useState("");
  const [recentGroups, setRecentGroups] = useState(new Set<string>());
  const [showEmpty, setShowEmpty] = useState(() => {
    try {
      return localStorage.getItem("foliochest.show-empty-groups") !== "false";
    } catch {
      return true;
    }
  });
  const [query, setQuery] = useState(""),
    [bodySearch, setBodySearch] = useState(false),
    [checked, setChecked] = useState(new Map<string, Member>()),
    [dialog, setDialog] = useState<DialogState>(null),
    [mobile, setMobile] = useState(false),
    [detail, setDetail] = useState(false),
    [dirty, setDirty] = useState(false),
    [busy, setBusy] = useState(false),
    [editorEpoch, setEditorEpoch] = useState(0),
    [revision, setRevision] = useState(0);
  const catalog = useCatalog(view === "trash", revision),
    search = useBodySearch(
      query,
      bodySearch && view === "entries",
      catalog.tick,
    );
  useDirty(dirty);
  const refresh = () => setRevision((n) => n + 1);
  const guard = async (action: () => void) => {
    if (busy || task.busy) {
      client.notice("操作正在提交，请等待完成后再切换。");
      return;
    }
    const epoch = client.epoch;
    if (
      dirty &&
      !(await confirm({
        title: "放弃未保存的修改？",
        description: "当前编辑内容尚未保存。",
        label: "放弃修改",
        danger: true,
      }))
    )
      return;
    if (epoch !== client.epoch) return;
    if (dirty) {
      setDirty(false);
      setEditorEpoch((n) => n + 1);
    }
    action();
  };
  const go = (next: View) =>
    void guard(() => {
      setView(next);
      history.replaceState(
        null,
        "",
        location.pathname +
          location.search +
          (next === "approvals" ? "#approvals" : ""),
      );
      setSelected(null);
      setIsNew(false);
      setDetail(false);
      setQuery("");
      setChecked(new Map());
      setMobile(false);
    });
  useEffect(() => {
    const followRequest = () => {
      if (location.hash === "#approvals") go("approvals");
    };
    window.addEventListener("hashchange", followRequest);
    return () => window.removeEventListener("hashchange", followRequest);
  });
  const open = (next: DialogState) =>
    void guard(() => {
      setDialog(next);
      setMobile(false);
    });
  const changed = (key: string, deleted = false) => {
    setDirty(false);
    setIsNew(false);
    setView(deleted ? "trash" : "entries");
    setQuery("");
    setChecked(new Map());
    setSelected(deleted ? null : key);
    setDetail(!deleted);
    refresh();
  };
  const lock = () =>
    void guard(
      () =>
        void task.run(async (ctx) => {
          await ctx.request("/api/vault/lock", {});
          await ctx.refresh();
        }),
    );
  const createEntry = (group = "") =>
    void guard(() => {
      setView("entries");
      history.replaceState(null, "", location.pathname + location.search);
      setQuery("");
      setChecked(new Map());
      setMobile(false);
      setDraftGroup(group);
      setIsNew(true);
      setSelected(null);
      setEditorEpoch((n) => n + 1);
      setDetail(true);
    });
  const sidebar = (
    <Sidebar
      directory={state.vault.directory}
      count={state.vault.entries}
      groups={catalog.groups}
      view={view}
      notice={
        <ApprovalNotice
          count={catalog.requests.length}
          onOpen={() => go("approvals")}
        />
      }
      onGo={go}
      onNew={() => createEntry()}
      onLock={lock}
      onQuit={() => void guard(() => void exit.quit())}
      onOpen={(action) => open({ kind: "vault", action })}
    >
      {(view === "entries" || view === "trash") && (
        <EntryList
          entries={filterEntries(catalog.entries, null, query, search.matches)}
          groups={catalog.groups}
          recentGroups={recentGroups}
          showEmpty={showEmpty}
          onShowEmpty={(value) => {
            setShowEmpty(value);
            try {
              localStorage.setItem(
                "foliochest.show-empty-groups",
                String(value),
              );
            } catch {
              /* Preference remains available for this session. */
            }
          }}
          onGroupCreated={(name) => {
            setRecentGroups((before) => new Set([...before, name]));
            setQuery("");
            refresh();
          }}
          query={query}
          bodySearch={bodySearch}
          trash={view === "trash"}
          selected={selected}
          checked={checked}
          format={state.vault.format}
          loading={catalog.loading || search.loading}
          error={search.error}
          onQuery={setQuery}
          onBodySearch={setBodySearch}
          onSelect={(key) =>
            void guard(() => {
              if (selected === key && !isNew) {
                setDetail(true);
                setMobile(false);
                return;
              }
              setMobile(false);
              setSelected(key);
              setIsNew(false);
              setDetail(true);
            })
          }
          onCheck={(entry, value) =>
            setChecked((before) => {
              const next = new Map(before);
              if (value)
                next.set(entry.key, {
                  key: entry.key,
                  expected_revision: entry.revision,
                });
              else next.delete(entry.key);
              return next;
            })
          }
          onNew={createEntry}
          onMove={() => open({ kind: "group" })}
          onManage={(name) => {
            const source =
              catalog.groups.find((g) => g.name === name) ??
              (recentGroups.has(name)
                ? { name, count: 0, deleted_count: 0, blocked: 0, members: [] }
                : undefined);
            if (source) open({ kind: "group", source });
          }}
          onClear={() => setChecked(new Map())}
        />
      )}
    </Sidebar>
  );
  return (
    <>
      <div className="app-layout" data-view={view} data-detail-open={detail}>
        <div className="desktop-sidebar">{sidebar}</div>
        <main className="main-shell">
          <div className="mobile-navigation">
            <IconButton label="打开导航" onClick={() => setMobile(true)}>
              <Menu />
            </IconButton>
            <ApprovalNotice
              count={catalog.requests.length}
              onOpen={() => go("approvals")}
            />
          </div>
          {(connectionError || catalog.error || task.error || exit.error) && (
            <div className="workspace-warning">
              <ErrorNotice
                message={
                  connectionError || catalog.error || task.error || exit.error
                }
              />
            </div>
          )}
          {!!state.vault.issues.length && (
            <details className="workspace-warning">
              <summary>{state.vault.issues.length} 项同步问题</summary>
              <ul className="issue-list scroll-region">
                {state.vault.issues.map((issue, i) => (
                  <li key={i}>{issue}</li>
                ))}
              </ul>
            </details>
          )}
          {state.system.enabled && !state.system.available && (
            <details className="workspace-warning">
              <summary>系统锁定监测不可用</summary>
              <p>{state.system.message}</p>
            </details>
          )}
          {(view === "entries" || view === "trash") && (
            <div className="workspace" data-detail-open={detail}>
              {selected !== null || isNew ? (
                <EntryEditor
                  key={
                    (isNew ? "new:" : "entry:" + selected) + ":" + editorEpoch
                  }
                  entryKey={isNew ? null : selected}
                  summary={catalog.entries.find((e) => e.key === selected)}
                  groups={catalog.groups}
                  format={state.vault.format}
                  initialGroup={draftGroup}
                  onDirty={setDirty}
                  onBusy={setBusy}
                  onSaved={(key) => changed(key)}
                  onCancel={() =>
                    void guard(() => {
                      if (isNew) {
                        setIsNew(false);
                        setSelected(null);
                        setDetail(false);
                      } else {
                        setEditorEpoch((n) => n + 1);
                      }
                    })
                  }
                  onManage={(kind, entry) => open({ kind, entry })}
                  onHistory={() => {
                    if (selected) open({ kind: "history", key: selected });
                  }}
                  onBack={() =>
                    void guard(() => {
                      setDetail(false);
                      setSelected(null);
                      setIsNew(false);
                    })
                  }
                />
              ) : (
                <div className="entry-panel">
                  <EmptyState
                    icon={<FileText />}
                    title={
                      view === "trash" ? "选择已删除的资料" : "选择一条资料"
                    }
                    action={
                      view === "entries" ? (
                        <Button variant="outline" onClick={() => createEntry()}>
                          新建资料
                        </Button>
                      ) : undefined
                    }
                  />
                </div>
              )}
            </div>
          )}
          {view === "settings" && (
            <Settings
              state={state}
              onOpen={(action) => open({ kind: "vault", action })}
            />
          )}
          {view === "approvals" && (
            <Approvals
              requests={catalog.requests}
              onRefresh={refresh}
              onBack={() => go("entries")}
            />
          )}
        </main>
      </div>
      <Dialog open={mobile} onOpenChange={setMobile}>
        <DialogContent className="navigation-drawer">
          <DialogTitle className="sr-only">导航</DialogTitle>
          <DialogDescription className="sr-only">
            展开分组、选择资料或打开设置
          </DialogDescription>
          {sidebar}
        </DialogContent>
      </Dialog>
      {dialog?.kind === "vault" && (
        <VaultFlow
          kind={dialog.action}
          state={state}
          onClose={() => setDialog(null)}
        />
      )}
      {dialog?.kind === "history" && (
        <HistoryDialog
          entryKey={dialog.key}
          format={state.vault.format}
          groups={catalog.groups}
          onClose={() => setDialog(null)}
          onChanged={changed}
        />
      )}
      {(dialog?.kind === "rename" || dialog?.kind === "delete") && (
        <EntryManagement
          kind={dialog.kind}
          entry={dialog.entry}
          onClose={() => setDialog(null)}
          onChanged={changed}
        />
      )}
      {dialog?.kind === "group" && (
        <GroupManagement
          source={dialog.source}
          members={[...checked.values()]}
          groups={catalog.groups}
          format={state.vault.format}
          onClose={() => setDialog(null)}
          onChanged={(next) => {
            if (dialog.source)
              setRecentGroups((before) => {
                const names = new Set(before);
                names.delete(dialog.source!.name);
                if (next) names.add(next);
                return names;
              });
            setChecked(new Map());
            setSelected(null);
            setIsNew(false);
            setDetail(false);
            refresh();
          }}
        />
      )}
    </>
  );
}
