import { useContext, useState } from "react";
import { Button } from "@/components/ui/button";
import { SettingRow } from "@/components/common";
import { ErrorNotice, useConfirm } from "@/components/forms";
import { ExitContext } from "@/components/exit-app";
import { useClient, useTask } from "@/lib/hooks";

type Update = {
  latest: string;
  status: "available" | "current" | "ahead" | "unknown";
  url: string;
  installer?: string;
  size?: number;
  notes?: string;
};
type Ready = { id: string; version: string; name: string; size: number; path: string };
const releasePage = "https://github.com/shenjiecode/foliochest/releases/latest";
const sizeText = (size: number) => (size / 1024 / 1024).toFixed(1) + " MiB";

export function UpdateSettings({ version }: { version: string }) {
  const [result, setResult] = useState<Update | null>(null);
  const [ready, setReady] = useState<Ready | null>(null);
  const [action, setAction] = useState("");
  const task = useTask(), confirm = useConfirm(), exited = useContext(ExitContext), client = useClient();
  const downloadUpdate = () => task.run(async (ctx) => {
    if (!result || !(await confirm({
      title: "下载新版安装包？",
      description: `将从 GitHub 下载 ${result.installer}（${sizeText(result.size || 0)}）并进行 SHA-256 完整性校验。`,
      label: "下载并校验",
    }))) return null;
    setAction("正在下载并校验…");
    setReady(null);
    return ctx.request<Ready>("/api/app/download-update", { version: result.latest, confirmed: true });
  }, (data) => { if (data) setReady(data); });
  const install = () => task.run(async (ctx) => {
    if (!ready || !(await confirm({
      title: "退出并启动安装器？",
      description: "更新前请保存正在编辑的内容。确认后将锁定资料库并退出程序，随后启动系统安装器；安装完成后重新打开资料匣即可。",
      label: "退出并更新",
    }))) return;
    setAction("正在准备退出…");
    const resume = client.pauseRefresh();
    try {
      await ctx.request("/api/app/install-update", { id: ready.id, confirmed: true });
      exited(`已启动系统安装器，请按提示完成安装后重新打开资料匣。如未弹出安装窗口，可手动运行：${ready.path}`);
    } catch (error) {
      resume();
      await ctx.refresh("未能启动安装器，可手动运行已下载的安装包。");
      throw error;
    } finally { resume(); }
  });
  return (
    <>
      <SettingRow title="程序版本" note={version}>
        <Button variant="outline" disabled={task.busy} onClick={() => {
          setResult(null); setReady(null); setAction("正在检查…");
          void task.run((ctx) => ctx.request<Update>("/api/app/check-update", { confirmed: true }), setResult);
        }}>
          {task.busy ? action || "正在处理…" : "检查更新"}
        </Button>
      </SettingRow>
      <p className="settings-footnote">仅在手动点击时检查 GitHub 新版本。</p>
      <ErrorNotice message={task.error} />
      {result && <>
        <p role="status" className="field-note">
          {result.status === "available" ? `发现新版本 ${result.latest}。` : result.status === "current" ? `当前已是最新正式版（${result.latest}）。` : result.status === "ahead" ? `当前版本高于最新正式版 ${result.latest}，无需降级。` : `最新正式版为 ${result.latest}；当前为无法比较的构建，请手动核对。`}
        </p>
        {result.notes && <details><summary>更新说明</summary><pre className="path-value">{result.notes}</pre></details>}
        {result.status === "available" && (result.installer ? <>
          <p className="field-note">{result.installer} · {sizeText(result.size || 0)}</p>
          {!ready && <Button disabled={task.busy} onClick={() => void downloadUpdate()}>{task.busy ? action : "下载安装包"}</Button>}
        </> : <p className="field-note">当前系统没有匹配的安装包，请打开下载页手动选择。</p>)}
      </>}
      {ready && <>
        <p role="status" className="field-note">安装包已下载并通过 SHA-256 校验，可随时开始更新。</p>
        <p className="path-value">{ready.path}</p>
        <Button disabled={task.busy} onClick={() => void install()}>退出并更新</Button>
      </>}
      <Button variant="outline" asChild>
        <a href={result?.url || releasePage} target="_blank" rel="noopener noreferrer" referrerPolicy="no-referrer">{result ? "打开 GitHub 版本下载页" : "打开 GitHub 下载页"}</a>
      </Button>
      <p className="settings-footnote">更新会保留现有资料库与设置。多设备同步使用时，建议一并更新。</p>
    </>
  );
}
