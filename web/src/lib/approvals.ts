import { useCallback, useEffect, useState } from "react";
import { useClient, useSession } from "./hooks";
import { errorText, StaleRequest } from "./client";
import type { Pending } from "./types";

// Approval status must not depend on entry/group loading or which view is open.
export function useApprovals() {
  const client = useClient();
  const { epoch } = useSession();
  const [revision, setRevision] = useState(0);
  const refresh = useCallback(() => setRevision((n) => n + 1), []);
  const [snapshot, setSnapshot] = useState({
    client, epoch, requests: [] as Pending[], error: "", loading: true,
  });
  useEffect(() => {
    let live = true, pending = false;
    const load = async () => {
      if (pending) return;
      pending = true;
      try {
        const requests = await client.request<Pending[]>("/api/approvals", undefined, epoch);
        if (!Array.isArray(requests)) throw new Error("审批状态响应无效");
        if (live) setSnapshot({ client, epoch, requests, error: "", loading: false });
      } catch (error) {
        if (live && !(error instanceof StaleRequest)) {
          setSnapshot((before) => ({
            client, epoch,
            requests: before.client === client && before.epoch === epoch ? before.requests : [],
            error: "审批状态暂未更新：" + errorText(error), loading: false,
          }));
        }
      } finally { pending = false; }
    };
    setSnapshot((before) => before.client === client && before.epoch === epoch
      ? { ...before, loading: true }
      : { client, epoch, requests: [], error: "", loading: true });
    void load();
    const timer = setInterval(() => void load(), 3000);
    const focus = () => { if (document.visibilityState !== "hidden") void load(); };
    window.addEventListener("focus", focus);
    document.addEventListener("visibilitychange", focus);
    return () => {
      live = false;
      clearInterval(timer);
      window.removeEventListener("focus", focus);
      document.removeEventListener("visibilitychange", focus);
    };
  }, [client, epoch, revision]);
  const current = snapshot.client === client && snapshot.epoch === epoch;
  return {
    requests: current ? snapshot.requests : [],
    error: current ? snapshot.error : "",
    loading: current ? snapshot.loading : true,
    refresh,
  };
}
