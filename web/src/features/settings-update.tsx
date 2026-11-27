import { useState } from "react";
import { Button } from "@/components/ui/button";
import { SettingRow } from "@/components/common";
import { ErrorNotice } from "@/components/forms";
import { useTask } from "@/lib/hooks";

type Update = {
  latest: string;
  status: "available" | "current" | "ahead" | "unknown";
  url: string;
};
const releasePage = "https://github.com/shenjiecode/foliochest/releases/latest";

export function UpdateSettings({ version }: { version: string }) {
  const [result, setResult] = useState<Update | null>(null);
  const task = useTask();
  return (
    <>
      <SettingRow title="程序版本" note={version}>
        <Button
          variant="outline"
          disabled={task.busy}
          onClick={() => {
            setResult(null);
            void task.run(
              (ctx) => ctx.request<Update>("/api/app/check-update", { confirmed: true }),
              setResult,
            );
          }}
        >
          {task.busy ? "正在检查…" : "检查更新"}
        </Button>
      </SettingRow>
      <p className="settings-footnote">
        仅点击时连接 GitHub 查询最新正式版，不上传资料或凭据，不后台检查、不自动下载安装。
      </p>
      <ErrorNotice message={task.error} />
      {result && (
        <p role="status" className="field-note">
          {result.status === "available"
            ? `发现新版本 ${result.latest}。`
            : result.status === "current"
              ? `当前已是最新正式版（${result.latest}）。`
              : result.status === "ahead"
                ? `当前版本高于最新正式版 ${result.latest}，无需降级。`
                : `最新正式版为 ${result.latest}；当前为无法比较的构建，请手动核对。`}
        </p>
      )}
      <Button variant="outline" asChild>
        <a
          href={result?.url || releasePage}
          target="_blank"
          rel="noopener noreferrer"
          referrerPolicy="no-referrer"
        >
          {result ? "打开 GitHub 版本下载页" : "打开 GitHub 下载页"}
        </a>
      </Button>
      <p className="settings-footnote">
        更新前先备份并退出资料匣，再安装对应系统的新包；双机同步请一起更新。
      </p>
    </>
  );
}
