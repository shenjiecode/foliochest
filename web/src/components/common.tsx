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
  children,
}: {
  title: string;
  note?: string;
  children: ReactNode;
}) {
  return (
    <div className="setting-row">
      <div>
        <h3>{title}</h3>
        {note && <p>{note}</p>}
      </div>
      {children}
    </div>
  );
}

export function Brand() {
  return (
    <span className="wordmark">
      <span className="wordmark-cn">资料匣</span>
      <span className="wordmark-en">FolioChest</span>
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
  children,
  action,
}: {
  icon: ReactNode;
  title: string;
  children?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div className="empty-state">
      <div className="empty-icon">{icon}</div>
      <h2>{title}</h2>
      {children && <p>{children}</p>}
      {action}
    </div>
  );
}
