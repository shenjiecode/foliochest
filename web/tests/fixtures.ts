import type { AppState, Entry, Summary } from "../src/lib/types";
export const state: AppState = {
  authenticated: true,
  csrf: "synthetic-csrf",
  version: "synthetic",
  device_notice: "",
  system: {
    available: true,
    enabled: true,
    locked: false,
    sleeping: false,
    message: "",
    lock_reason: "",
  },
  vault: {
    format: 3,
    directory: "/synthetic/vault",
    exists: true,
    unlocked: true,
    entries: 1,
    issues: [],
  },
};
export const entry: Entry = {
  entry_id: "a".repeat(32),
  key: "synthetic/key",
  description: "Synthetic description",
  group: "工作",
  revision: "b".repeat(32),
  read_policy: "ask",
  write_policy: "deny",
  updated_at: "2026-01-01T00:00:00Z",
  value: "SYNTHETIC_PRIVATE_BODY",
  format: "markdown",
  attachments: [],
};
export const summary: Summary = { ...entry, state: "ready", versions: 1 };
export const json = (data: unknown, status = 200) =>
  new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json" },
  });
export function deferred<T>() {
  let resolve!: (value: T) => void, reject!: (error: unknown) => void;
  const promise = new Promise<T>((a, b) => {
    resolve = a;
    reject = b;
  });
  return { promise, resolve, reject };
}
