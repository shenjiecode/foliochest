import type { AppState } from "./types";
export class StaleRequest extends Error {
  constructor() {
    super("stale");
  }
}
export const errorText = (error: unknown) =>
  error instanceof Error ? error.message : "操作失败，请重试";
export class Sequence {
  private value = 0;
  next() {
    return ++this.value;
  }
  current(ticket: number) {
    return ticket === this.value;
  }
}
type Snapshot = {
  state: AppState | null;
  epoch: number;
  error: string;
  notice: string;
};
export class SessionClient {
  private snapshot: Snapshot = { state: null, epoch: 0, error: "", notice: "" };
  private listeners = new Set<() => void>();
  private states = new Sequence();
  private refreshPauses = 0;
  // A controlled shutdown must not lose its response to a concurrent lock-state poll.
  pauseRefresh() {
    this.refreshPauses++;
    this.states.next();
    let resumed = false;
    return () => {
      if (!resumed) { this.refreshPauses--; resumed = true; }
    };
  }
  constructor(private transport: typeof fetch = (...args) => fetch(...args)) {}
  getSnapshot = () => this.snapshot;
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };
  private publish(next: Snapshot) {
    this.snapshot = next;
    this.listeners.forEach((fn) => fn());
  }
  get epoch() {
    return this.snapshot.epoch;
  }
  assert(epoch: number) {
    if (epoch !== this.epoch) throw new StaleRequest();
  }
  notice(text: string) {
    this.publish({ ...this.snapshot, notice: text });
  }
  invalidate() {
    this.states.next();
    this.publish({
      ...this.snapshot,
      epoch: this.epoch + 1,
      notice: "",
      state: this.snapshot.state
        ? { ...this.snapshot.state, authenticated: false }
        : null,
    });
  }
  async refresh(notice?: string): Promise<void> {
    if (this.refreshPauses > 0) return;
    const ticket = this.states.next(),
      epoch = this.epoch;
    try {
      const response = await this.transport("/api/state", {
        credentials: "same-origin",
        cache: "no-store",
        signal: AbortSignal.timeout(10000),
      });
      const state = (await response.json()) as AppState;
      if (!this.states.current(ticket) || epoch !== this.epoch) return;
      if (
        !response.ok ||
        typeof state.csrf !== "string" ||
        typeof state.authenticated !== "boolean" ||
        !state.vault
      )
        throw new Error("无法读取资料库状态，请确认本机服务仍在运行");
      const before = this.snapshot.state;
      const changed =
        !!before &&
        (before.csrf !== state.csrf ||
          before.authenticated !== state.authenticated ||
          before.vault.directory !== state.vault.directory);
      this.publish({
        state,
        epoch: epoch + Number(changed),
        error: "",
        notice:
          notice ??
          (changed
            ? before?.authenticated
              ? "会话已更新，页面已重置。"
              : ""
            : this.snapshot.notice),
      });
    } catch (error) {
      if (this.states.current(ticket) && epoch === this.epoch)
        this.publish({ ...this.snapshot, error: errorText(error) });
    }
  }
  async request<T>(
    path: string,
    body?: unknown,
    epoch = this.epoch,
  ): Promise<T> {
    const response = await this.response(path, body, epoch);
    const data = await response.json();
    this.assert(epoch);
    return data as T;
  }
  async blob(path: string, body: unknown, epoch = this.epoch): Promise<Blob> {
    const response = await this.response(path, body, epoch);
    const blob = await response.blob();
    this.assert(epoch);
    return blob;
  }
  private async response(path: string, body: unknown, epoch: number) {
    this.assert(epoch);
    let response: Response;
    try {
      response = await this.transport(path, {
        method: body === undefined ? "GET" : "POST",
        signal: AbortSignal.timeout(120000),
        credentials: "same-origin",
        cache: "no-store",
        headers:
          body === undefined
            ? {}
            : {
                "Content-Type": "application/json",
                "X-FolioChest-CSRF": this.snapshot.state?.csrf ?? "",
              },
        body: body === undefined ? undefined : JSON.stringify(body),
      });
    } catch {
      this.assert(epoch);
      throw new Error("无法连接本机服务，请确认资料匣正在运行。");
    }
    this.assert(epoch);
    if (!response.ok) {
      const data = await response.json().catch(() => ({}));
      this.assert(epoch);
      if (response.status === 401 && this.refreshPauses === 0) {
        this.invalidate();
        void this.refresh();
      }
      throw new Error(data.error || `请求失败（HTTP ${response.status}）`);
    }
    return response;
  }
}
