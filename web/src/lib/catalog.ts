import { useEffect, useRef, useState } from "react";
import { useClient } from "./hooks";
import { errorText, Sequence, StaleRequest } from "./client";
import type { Group, Summary } from "./types";
export function useCatalog(trash: boolean, revision: number) {
  const client = useClient(),
    sequence = useRef(new Sequence());
  const [snapshot, setSnapshot] = useState({
    trash,
    entries: [] as Summary[],
    groups: [] as Group[],
    tick: 0,
  });
  const [error, setError] = useState(""),
    [loading, setLoading] = useState(true);
  useEffect(() => {
    let live = true,
      pending = false;
    setLoading(true);
    const load = async () => {
      if (pending) return;
      pending = true;
      const ticket = sequence.current.next();
      try {
        const [entries, groups] = await Promise.all([
          client.request<Summary[]>(
            "/api/entries" + (trash ? "?scope=trash" : ""),
          ),
          client.request<Group[]>("/api/groups"),
        ]);
        if (live && sequence.current.current(ticket)) {
          setSnapshot((before) => ({
            trash,
            entries: entries || [],
            groups: groups || [],
            tick: before.tick + 1,
          }));
          setError("");
        }
      } catch (e) {
        if (live && !(e instanceof StaleRequest)) setError(errorText(e));
      } finally {
        pending = false;
        if (live) setLoading(false);
      }
    };
    void load();
    const timer = setInterval(() => void load(), 3000);
    return () => {
      live = false;
      sequence.current.next();
      clearInterval(timer);
    };
  }, [client, trash, revision]);
  return {
    ...snapshot,
    entries: snapshot.trash === trash ? snapshot.entries : [],
    error,
    loading,
  };
}
export function useBodySearch(
  query: string,
  enabled: boolean,
  revision: number,
) {
  const client = useClient(),
    [result, setResult] = useState<{ query: string; keys: Set<string> } | null>(
      null,
    ),
    [error, setError] = useState("");
  useEffect(() => {
    let live = true;
    setError("");
    if (!enabled || !query.trim()) {
      setResult(null);
      return;
    }
    const timer = setTimeout(() => {
      void client
        .request<Summary[]>(
          "/api/entries?scope=body&q=" + encodeURIComponent(query),
        )
        .then((entries) => {
          if (live)
            setResult({
              query,
              keys: new Set((entries || []).map((e) => e.key)),
            });
        })
        .catch((e) => {
          if (live && !(e instanceof StaleRequest)) setError(errorText(e));
        });
    }, 250);
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [client, query, enabled, revision]);
  return {
    matches:
      enabled && query.trim()
        ? result?.query === query
          ? result.keys
          : new Set<string>()
        : null,
    error,
    loading: enabled && !!query.trim() && result?.query !== query && !error,
  };
}
