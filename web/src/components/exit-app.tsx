import { createContext, useContext } from "react";
import { useConfirm } from "@/components/forms";
import { useTask } from "@/lib/hooks";

export const ExitContext = createContext<(message?: string) => void>(() => {});
export function useQuit() {
  const confirm = useConfirm();
  const task = useTask();
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
    await task.run(
      (ctx) => ctx.request("/api/app/quit", { confirmed: true }),
      () => exited(),
    );
  };
  return { quit, busy: task.busy, error: task.error };
}
