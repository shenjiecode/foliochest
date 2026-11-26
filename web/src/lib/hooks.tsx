import {
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import { SessionClient, StaleRequest, errorText } from "./client";
export const ClientContext = createContext<SessionClient | null>(null);
export function useClient() {
  const client = useContext(ClientContext);
  if (!client) throw new Error("缺少网页会话");
  return client;
}
export function useSession() {
  const client = useClient();
  return useSyncExternalStore(client.subscribe, client.getSnapshot);
}
export type TaskContext = {
  current: () => boolean;
  assert: () => void;
  request: <T>(path: string, body?: unknown) => Promise<T>;
  blob: (path: string, body: unknown) => Promise<Blob>;
  refresh: (notice?: string) => Promise<void>;
};
export function useTask() {
  const client = useClient(),
    alive = useRef(true),
    pending = useRef(false);
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);
  async function run<T>(
    action: (ctx: TaskContext) => Promise<T>,
    success?: (result: T) => void,
  ) {
    if (pending.current || !alive.current) return;
    pending.current = true;
    setBusy(true);
    setError("");
    const epoch = client.epoch;
    const current = () => alive.current && epoch === client.epoch;
    const assert = () => {
      if (!current()) throw new StaleRequest();
    };
    const ctx: TaskContext = {
      current,
      assert,
      request: <R,>(path: string, body?: unknown) => {
        assert();
        return client.request<R>(path, body, epoch);
      },
      blob: (path, body) => {
        assert();
        return client.blob(path, body, epoch);
      },
      refresh: (notice) => {
        assert();
        return client.refresh(notice);
      },
    };
    try {
      const result = await action(ctx);
      if (current()) success?.(result);
    } catch (e) {
      if (current() && !(e instanceof StaleRequest)) setError(errorText(e));
    } finally {
      pending.current = false;
      if (alive.current) setBusy(false);
    }
  }
  return { busy, error, run, clearError: () => setError("") };
}
export function useResource<T>(path: string | null, revision = 0) {
  const client = useClient();
  const [result, setResult] = useState<{
    path: string | null;
    revision: number;
    client: SessionClient;
    data: T | null;
    error: string;
    loading: boolean;
  }>({
    path: null,
    revision: -1,
    client,
    data: null,
    error: "",
    loading: true,
  });
  useEffect(() => {
    let live = true;
    const base = {
      path,
      revision,
      client,
      data: null,
      error: "",
      loading: !!path,
    };
    setResult(base);
    if (!path) return;
    client
      .request<T>(path)
      .then((data) => {
        if (live) setResult({ ...base, data, loading: false });
      })
      .catch((error) => {
        if (live && !(error instanceof StaleRequest))
          setResult({ ...base, error: errorText(error), loading: false });
      });
    return () => {
      live = false;
    };
  }, [client, path, revision]);
  // Do not render the previous document for even one commit while a new effect
  // is being scheduled: otherwise input may land in the outgoing editor.
  const current =
    result.path === path &&
    result.revision === revision &&
    result.client === client;
  return current ? result : { data: null, error: "", loading: !!path };
}
export function useDirty(dirty: boolean) {
  useEffect(() => {
    if (!dirty) return;
    const handler = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = "";
    };
    window.addEventListener("beforeunload", handler);
    return () => window.removeEventListener("beforeunload", handler);
  }, [dirty]);
}
