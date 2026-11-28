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
import { ChevronDown } from "lucide-react";
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
  const [open, setOpen] = useState(false);
  const [filter, setFilter] = useState("");
  const [active, setActive] = useState(-1);
  const input = useRef<HTMLInputElement>(null);
  const unavailable = disabled || format < 3;
  const options = [
    { name: "", label: "未分组" },
    ...groups
      .filter((g) => g.name)
      .map((g) => ({ name: g.name, label: g.name })),
  ].filter((g) =>
    g.label.toLocaleLowerCase().includes(filter.toLocaleLowerCase()),
  );
  const expanded = open && !unavailable;
  const choose = (name: string) => {
    onChange(name);
    setOpen(false);
    setActive(-1);
  };
  return (
    <Field
      id={id}
      label="分组"
      note={
        format < 3
          ? "分组需要格式 3，可在设置中备份并升级。"
          : compact
            ? undefined
            : "留空为未分组"
      }
    >
      <div
        className="group-combobox"
        onBlur={(e) => {
          if (!e.currentTarget.contains(e.relatedTarget as Node | null))
            setOpen(false);
        }}
      >
        <Input
          ref={input}
          id={id}
          role="combobox"
          aria-expanded={expanded}
          aria-controls={expanded ? id + "-options" : undefined}
          aria-autocomplete="list"
          aria-activedescendant={
            expanded && active >= 0 && active < options.length
              ? id + "-option-" + active
              : undefined
          }
          autoComplete="off"
          value={value}
          onFocus={() => {
            setFilter("");
            setActive(-1);
            setOpen(true);
          }}
          onChange={(e) => {
            onChange(e.target.value);
            setFilter(e.target.value);
            setActive(-1);
            setOpen(true);
          }}
          onKeyDown={(e) => {
            if (e.key === "Escape" && expanded) {
              e.preventDefault();
              e.stopPropagation();
              setOpen(false);
            } else if (e.key === "ArrowDown" || e.key === "ArrowUp") {
              e.preventDefault();
              setOpen(true);
              setActive((n) =>
                options.length
                  ? e.key === "ArrowDown"
                    ? (n + 1) % options.length
                    : n <= 0
                      ? options.length - 1
                      : n - 1
                  : -1,
              );
            } else if (e.key === "Enter" && expanded) {
              e.preventDefault();
              if (active >= 0 && active < options.length)
                choose(options[active].name);
              else setOpen(false);
            }
          }}
          disabled={unavailable}
          placeholder="选择已有组或输入组名"
        />
        <button
          type="button"
          className="group-toggle"
          aria-label="展开分组选项"
          disabled={unavailable}
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => {
            if (expanded) setOpen(false);
            else {
              input.current?.focus();
              setFilter("");
              setActive(-1);
              setOpen(true);
            }
          }}
        >
          <ChevronDown />
        </button>
        {expanded && (
          <div
            className="group-options"
            id={id + "-options"}
            role="listbox"
            aria-label="分组选项"
          >
            {options.map((g, index) => (
              <button
                type="button"
                role="option"
                tabIndex={-1}
                id={id + "-option-" + index}
                key={g.name}
                aria-selected={g.name === value}
                data-active={index === active}
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => choose(g.name)}
              >
                {g.label}
              </button>
            ))}
            {!options.length && (
              <p className="field-note">将使用输入的新组名</p>
            )}
          </div>
        )}
      </div>
    </Field>
  );
}
