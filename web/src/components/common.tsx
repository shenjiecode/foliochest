import type { ReactNode, ComponentProps } from "react";
import { Button } from "@/components/ui/button";

import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";

export function SettingRow({
  title,
  note,
  icon,
  status,
  children,
}: {
  title: string;
  note?: string;
  icon?: ReactNode;
  status?: string;
  children: ReactNode;
}) {
  return (
    <div className="setting-row">
      {icon && (
        <span className="setting-icon" aria-hidden="true">
          {icon}
        </span>
      )}
      <div className="setting-copy">
        <h3>{title}</h3>
        {note && <p>{note}</p>}
      </div>
      <div className="setting-control">
        {status && <span className="setting-state">{status}</span>}
        {children}
      </div>
    </div>
  );
}

export function Brand() {
  return (
    <span className="brand">
      <img
        className="brand-logo"
        src="/logo.png"
        width="48"
        height="48"
        alt=""
      />
      <span className="wordmark">
        <span className="wordmark-cn">资料匣</span>
        <span className="wordmark-en">FolioChest</span>
      </span>
    </span>
  );
}
export function IconButton({
  label,
  children,
  className,
  ...props
}: ComponentProps<typeof Button> & { label: string }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          aria-label={label}
          className={cn("icon-button", className)}
          {...props}
        >
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
export function Field({
  id,
  label,
  children,
  note,
}: {
  id: string;
  label: string;
  children: ReactNode;
  note?: string;
}) {
  return (
    <div className="field">
      <Label htmlFor={id}>{label}</Label>
      {children}
      {note && <p className="field-note">{note}</p>}
    </div>
  );
}
export function EmptyState({
  icon,
  title,
  action,
}: {
  icon: ReactNode;
  title: string;
  action?: ReactNode;
}) {
  return (
    <div className="empty-state">
      <div className="empty-icon">{icon}</div>
      <h2>{title}</h2>
      {action}
    </div>
  );
}
