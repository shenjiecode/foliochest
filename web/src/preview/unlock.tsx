import { ArrowRight, FolderClosed, LockKeyhole } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Brand, Field, PreviewBadge } from "./shared";
import type { DialogKind } from "./model";
export function Unlock({
  onUnlock,
  onOpen,
}: {
  onUnlock: () => void;
  onOpen: (kind: DialogKind) => void;
}) {
  return (
    <main className="lock-screen">
      <header className="lock-topbar">
        <Brand />
        <PreviewBadge />
      </header>
      <div className="lock-center scroll-region">
        <section className="unlock-card">
          <div className="vault-emblem">
            <LockKeyhole />
          </div>
          <h1>解锁资料库</h1>
          <div className="unlock-vault">
            <FolderClosed />
            <span>个人资料</span>
          </div>
          <form
            onSubmit={(event) => {
              event.preventDefault();
              onUnlock();
            }}
          >
            <Field id="preview-password" label="主密码">
              <Input
                id="preview-password"
                type="password"
                autoComplete="off"
                readOnly
                value=""
                placeholder="界面预览，无需输入密码"
              />
            </Field>
            <Button type="submit" className="unlock-submit">
              解锁
              <ArrowRight />
            </Button>
          </form>
          <div className="unlock-links">
            <button onClick={() => onOpen("open-vault")}>打开其他资料库</button>
            <button onClick={() => onOpen("create-vault")}>创建资料库</button>
            <button onClick={() => onOpen("restore-vault")}>从备份恢复</button>
          </div>
        </section>
      </div>
    </main>
  );
}
