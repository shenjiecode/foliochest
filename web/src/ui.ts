export type API = <T>(path: string, body?: unknown) => Promise<T>;
export const esc = (value: unknown): string =>
  String(value ?? "").replace(
    /[&<>"']/g,
    (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        c
      ]!,
  );
export function download(blob: Blob, name: string): void {
  const href = URL.createObjectURL(blob),
    link = document.createElement("a");
  link.href = href;
  link.download = name;
  link.click();
  setTimeout(() => URL.revokeObjectURL(href), 1000);
}
export function encodeBase64(bytes: Uint8Array): string {
  let binary = "";
  for (let i = 0; i < bytes.length; i += 16384)
    binary += String.fromCharCode(...bytes.subarray(i, i + 16384));
  return btoa(binary);
}
export async function base64(file: File): Promise<string> {
  return encodeBase64(new Uint8Array(await file.arrayBuffer()));
}
export function downloadAttachment(file: { name: string; data: string }): void {
  download(
    new Blob([Uint8Array.from(atob(file.data), (c) => c.charCodeAt(0))], {
      type: "application/octet-stream",
    }),
    file.name,
  );
}

type Control =
  | HTMLInputElement
  | HTMLButtonElement
  | HTMLSelectElement
  | HTMLTextAreaElement;
const busyHosts = new WeakMap<
  HTMLElement,
  {
    pending: number;
    aria: string | null;
    controls: Map<Control, boolean>;
  }
>();

export async function withBusy<T>(
  host: HTMLElement,
  action: () => Promise<T>,
): Promise<T> {
  let state = busyHosts.get(host);
  if (!state) {
    state = {
      pending: 0,
      aria: host.getAttribute("aria-busy"),
      controls: new Map(),
    };
    busyHosts.set(host, state);
  }
  state.pending++;
  for (const el of host.querySelectorAll<Control>(
    "input,button,select,textarea",
  )) {
    if (!state.controls.has(el)) state.controls.set(el, el.disabled);
    el.disabled = true;
  }
  host.setAttribute("aria-busy", "true");
  try {
    return await action();
  } finally {
    // Overlapping submissions share the original state until both finish.
    if (--state.pending === 0) {
      busyHosts.delete(host);
      if (state.aria === null) host.removeAttribute("aria-busy");
      else host.setAttribute("aria-busy", state.aria);
      state.controls.forEach((disabled, el) => {
        if (el.isConnected) el.disabled = disabled;
      });
    }
  }
}
