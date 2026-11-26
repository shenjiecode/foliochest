import type { ReactNode, ComponentProps } from "react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";

export function Brand() {
  return (
    <span className="wordmark">
      FolioChest<span className="sr-only"> · 资料匣</span>
    </span>
  );
}
export function PreviewBadge() {
  return (
    <Badge variant="outline" className="preview-badge">
      界面预览 · 示例资料
    </Badge>
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
export function SelectField({
  className,
  children,
  ...props
}: ComponentProps<"select">) {
  return (
    <select className={cn("field-select", className)} {...props}>
      {children}
    </select>
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
