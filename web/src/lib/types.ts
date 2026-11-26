export type Policy = "allow" | "ask" | "deny";
export type Format = "text" | "markdown" | "json";
export type Attachment = { name: string; media_type: string; data: string };
export type Summary = {
  entry_id: string;
  key: string;
  description: string;
  group?: string;
  candidate_groups?: string[];
  revision: string;
  read_policy: Policy;
  write_policy: Policy;
  state: string;
  versions: number;
  deleted?: boolean;
};
export type Entry = Omit<Summary, "state" | "versions"> & {
  value: string;
  format: Format;
  attachments: Attachment[] | null;
  updated_at: string;
};
export type AppState = {
  authenticated: boolean;
  csrf: string;
  version: string;
  device_notice: string;
  system: {
    available: boolean;
    locked: boolean;
    sleeping: boolean;
    enabled: boolean;
    message: string;
    lock_reason: string;
  };
  vault: {
    format: number;
    directory: string;
    exists: boolean;
    unlocked: boolean;
    entries: number;
    issues: string[];
  };
};
export type Member = { key: string; expected_revision: string };
export type Group = {
  name: string;
  count: number;
  deleted_count: number;
  blocked: number;
  members: Member[];
};
export type HistoryVersion = Omit<Entry, "value" | "format" | "attachments"> & {
  head: boolean;
  available: boolean;
};
export type History = {
  key: string;
  state: string;
  heads: string[];
  versions: HistoryVersion[];
};
export type Pending = {
  id: string;
  client_name: string;
  operation: string;
  expires_at: string;
  reads?: {
    key: string;
    revision?: string;
    include_value: boolean;
    attachments?: string[];
    all_attachments?: boolean;
  }[];
  changes?: {
    key: string;
    expected_revision: string;
    value?: string;
    description?: string;
    format?: string;
    attachments?: Attachment[];
  }[];
};
export type Client = { id: string; name: string; created_at: string };
export type Security = {
  idle_lock_minutes: number;
  lock_on_system_events: boolean;
};
export type Device = {
  auto_unlock: boolean;
  matches_vault: boolean;
  remembered_browser: boolean;
  cleanup_pending: boolean;
  provider: string;
  startup: { supported: boolean; enabled: boolean; current: boolean };
  startup_error: string;
  notice: string;
};
export type VaultAction = "open" | "create" | "restore";
export const policyLabels: Record<Policy, string> = {
  allow: "免确认",
  ask: "需确认",
  deny: "拒绝",
};
export const stateLabels: Record<string, string> = {
  ready: "可用",
  conflict: "版本冲突",
  incomplete: "同步未完整",
  missing: "文件缺失",
  invalid: "校验失败",
  deleted: "已删除，可恢复",
};
export const vaultName = (directory: string) =>
  directory.split(/[\\/]/).filter(Boolean).pop() || directory;
