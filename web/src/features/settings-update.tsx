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
      description: `将从 GitHub 下载 ${result.installer}（${sizeText(result.size || 0)}）并校验 SHA-256。当前未提供发布者代码签名，校验和不是签名；请确认信任项目来源。此步骤不会退出程序或执行安装。`,
      label: "下载并校验",
    }))) return null;
    setAction("正在下载并校验…");
    setReady(null);
    return ctx.request<Ready>("/api/app/download-update", { version: result.latest, confirmed: true });
  }, (data) => { if (data) setReady(data); });
  const install = () => task.run(async (ctx) => {
    if (!ready || !(await confirm({
      title: "退出并启动安装器？",
      description: "请先保存编辑、导出备份并暂停其他设备写入。资料库将锁定，网页和 MCP 会断开。Windows 打开安装向导，Linux 交给系统软件安装器（可能要求系统认证）；无需先卸载。完成后请从应用菜单重新启动。",
      label: "退出并更新",
    }))) return;
    setAction("正在准备退出…");
    const resume = client.pauseRefresh();
    try {
      await ctx.request("/api/app/install-update", { id: ready.id, confirmed: true });
      exited(`正在交给系统安装器，这不代表安装已完成。请完成安装后从应用菜单重新启动资料匣。若未出现安装窗口，可手动安装已校验的文件：${ready.path}`);
    } catch (error) {
      resume();
      await ctx.refresh("更新未确认启动，请检查安装器是否已打开；资料库可能已锁定。已下载安装包保留，可手动安装。");
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
      <p className="settings-footnote">仅点击时连接 GitHub，不上传资料或凭据，不后台检查。下载和启动安装器分别确认，不静默提权或自动安装。</p>
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
        <p role="status" className="field-note">安装包已下载，SHA-256 校验通过。不会自动执行。</p>
        <p className="path-value">{ready.path}</p>
        <Button disabled={task.busy} onClick={() => void install()}>退出并更新</Button>
      </>}
      <Button variant="outline" asChild>
        <a href={result?.url || releasePage} target="_blank" rel="noopener noreferrer" referrerPolicy="no-referrer">{result ? "打开 GitHub 版本下载页" : "打开 GitHub 下载页"}</a>
      </Button>
      <p className="settings-footnote">程序与资料库分离，正常更新保留资料与本机配置；双机同步请一起更新。Linux 若未配置软件安装器，请手动安装下载的 RPM / DEB。</p>
    </>
  );
}
