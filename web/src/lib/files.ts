import type { Attachment } from "./types";
export function encodeBase64(bytes: Uint8Array): string {
  let binary = "";
  for (let i = 0; i < bytes.length; i += 16384)
    binary += String.fromCharCode(...bytes.subarray(i, i + 16384));
  return btoa(binary);
}
export async function readFile(
  file: File,
  max: number,
  assertCurrent: () => void,
): Promise<string> {
  assertCurrent();
  if (file.size > max)
    throw new Error(`文件 ${file.name} 超过 ${max / 1024 / 1024} MiB`);
  const bytes = new Uint8Array(await file.arrayBuffer());
  try {
    assertCurrent();
    if (bytes.length > max) throw new Error("文件过大");
    return encodeBase64(bytes);
  } finally {
    bytes.fill(0);
  }
}
export async function addAttachments(
  current: Attachment[],
  files: File[],
  assertCurrent: () => void,
) {
  if (current.length + files.length > 16)
    throw new Error("每条资料最多 16 个附件");
  const names = new Set(current.map((f) => f.name));
  for (const file of files) {
    if (names.has(file.name))
      throw new Error(`已存在同名附件：${file.name}，请先移除旧附件`);
    if (file.size > 2 * 1024 * 1024)
      throw new Error(`附件 ${file.name} 超过 2 MiB`);
    names.add(file.name);
  }
  const added: Attachment[] = [];
  for (const file of files)
    added.push({
      name: file.name,
      media_type: file.type || "application/octet-stream",
      data: await readFile(file, 2 * 1024 * 1024, assertCurrent),
    });
  assertCurrent();
  return [...current, ...added];
}
export function download(blob: Blob, name: string) {
  const url = URL.createObjectURL(blob),
    link = document.createElement("a");
  link.href = url;
  link.download = name;
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
export function downloadAttachment(file: Attachment) {
  download(
    new Blob([Uint8Array.from(atob(file.data), (c) => c.charCodeAt(0))], {
      type: "application/octet-stream",
    }),
    file.name,
  );
}
export const fileSize = (data: string) =>
  Math.max(
    0,
    Math.floor((data.length * 3) / 4) -
      (data.endsWith("==") ? 2 : data.endsWith("=") ? 1 : 0),
  );
