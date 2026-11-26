import {
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "./ui/dialog";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Field } from "./common";
import type { Group } from "@/lib/types";

export function ErrorNotice({ message }: { message?: string }) {
  return message ? (
    <p className="form-error" role="alert">
      {message}
    </p>
  ) : null;
}
export function Notice({ children }: { children: ReactNode }) {
  return <div className="dialog-notice">{children}</div>;
}
export function Modal({
  title,
  description,
  children,
  footer,
  onClose,
  busy = false,
  wide = false,
}: {
  title: string;
  description?: string;
  children: ReactNode;
  footer?: ReactNode;
  onClose: () => void;
  busy?: boolean;
  wide?: boolean;
}) {
  const [trigger] = useState(
    () => document.activeElement as HTMLElement | null,
  );
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !busy) onClose();
      }}
    >
      <DialogContent
        className={"dialog-card" + (wide ? " history-dialog" : "")}
        showCloseButton={!busy}
        onEscapeKeyDown={(e) => {
          if (busy) e.preventDefault();
        }}
        onPointerDownOutside={(e) => e.preventDefault()}
        onCloseAutoFocus={(e) => {
          e.preventDefault();
          if (trigger?.isConnected) trigger.focus();
          else
            document
              .querySelector<HTMLButtonElement>(
                ".mobile-menu, .vault-switch, .unlock-submit",
              )
              ?.focus();
        }}
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription className={description ? "" : "sr-only"}>
            {description || title}
          </DialogDescription>
        </DialogHeader>
        <div className="dialog-body">{children}</div>
        {footer && <DialogFooter>{footer}</DialogFooter>}
      </DialogContent>
    </Dialog>
  );
}
type Confirmation = {
  title: string;
  description: string;
  label?: string;
  danger?: boolean;
};
const ConfirmationContext = createContext<
  (options: Confirmation) => Promise<boolean>
>(() => Promise.resolve(false));
export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [options, setOptions] = useState<Confirmation | null>(null),
    resolver = useRef<((value: boolean) => void) | null>(null);
  useEffect(
    () => () => {
      resolver.current?.(false);
      resolver.current = null;
    },
    [],
  );
  const decide = (value: boolean) => {
    const resolve = resolver.current;
    resolver.current = null;
    setOptions(null);
    resolve?.(value);
  };
  const confirm = (next: Confirmation) =>
    new Promise<boolean>((resolve) => {
      if (resolver.current) {
        resolve(false);
        return;
      }
      resolver.current = resolve;
      setOptions(next);
    });
  return (
    <ConfirmationContext value={confirm}>
      {children}
      {options && (
        <Modal
          title={options.title}
          onClose={() => decide(false)}
          footer={
            <>
              <Button variant="outline" onClick={() => decide(false)}>
                取消
              </Button>
              <Button
                variant={options.danger ? "destructive" : "default"}
                onClick={() => decide(true)}
              >
                {options.label || "确认"}
              </Button>
            </>
          }
        >
          <p>{options.description}</p>
        </Modal>
      )}
    </ConfirmationContext>
  );
}
export const useConfirm = () => useContext(ConfirmationContext);
export function GroupField({
  value,
  onChange,
  groups,
  format,
  id = "entry-group",
  disabled = false,
  compact = false,
}: {
  value: string;
  onChange: (value: string) => void;
  groups: Group[];
  format: number;
  id?: string;
  disabled?: boolean;
  compact?: boolean;
}) {
  return (
    <Field
      id={id}
      label="分组"
      note={
        format < 3
          ? "分组需要格式 3，可在设置中备份并升级。"
          : compact
            ? undefined
            : "明文，留空为未分组"
      }
    >
      <Input
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        disabled={disabled || format < 3}
        list={id + "-options"}
        placeholder="选择已有组或输入组名"
      />
      <datalist id={id + "-options"}>
        {groups
          .filter((g) => g.name)
          .map((g) => (
            <option key={g.name} value={g.name} />
          ))}
      </datalist>
    </Field>
  );
}
