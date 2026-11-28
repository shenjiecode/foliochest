import { createContext, useContext } from "react";
import { useConfirm } from "@/components/forms";
import { useClient, useTask } from "@/lib/hooks";

export const ExitContext = createContext<(message?: string) => void>(() => {});
export function useQuit() {
  const confirm = useConfirm();
  const task = useTask();
  const client = useClient();
  const exited = useContext(ExitContext);
  const quit = async () => {
    if (
      task.busy ||
      !(await confirm({
        title: "退出资料匣？",
        description:
          "将锁定资料库并停止本机服务，AI 连接也会断开。下次点击应用图标可重新启动。",
        label: "退出",
        danger: true,
      }))
    )
      return;
    await task.run(async (ctx) => {
      const resume = client.pauseRefresh();
      try {
        await ctx.request("/api/app/quit", { confirmed: true });
        exited();
      } catch (error) {
        resume();
        if (ctx.current()) await ctx.refresh();
        throw error;
      }
    });
  };
  return { quit, busy: task.busy, error: task.error };
}
