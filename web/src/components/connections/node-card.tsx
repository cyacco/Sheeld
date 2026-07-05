"use client";

import Link from "next/link";
import { ArrowUpRight, Link2, Link2Off, type LucideIcon } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

export interface NodeCardProps {
  nodeId: string;
  title: string;
  subtitle?: string;
  icon: LucideIcon;
  href: string;
  enabled: boolean;
  badge?: string;
  selected: boolean;
  /** Node is a valid attach/detach target for the current selection. */
  targetable: boolean;
  /** When targetable: is it currently attached to the selection? */
  attached: boolean;
  registerRef: (el: HTMLElement | null) => void;
  onSelect: () => void;
  onToggleAttach: () => void;
}

export function NodeCard({
  title,
  subtitle,
  icon: Icon,
  href,
  enabled,
  badge,
  selected,
  targetable,
  attached,
  registerRef,
  onSelect,
  onToggleAttach,
}: NodeCardProps) {
  return (
    <div
      ref={registerRef}
      onClick={targetable ? onToggleAttach : onSelect}
      className={cn(
        "group flex cursor-pointer items-center gap-3 rounded-lg border bg-card p-3 transition-colors",
        selected && "border-primary ring-1 ring-primary",
        targetable && "hover:border-primary/60",
        !targetable && !selected && "hover:bg-accent/50",
        !enabled && "opacity-60",
      )}
    >
      <div className="rounded-md bg-primary/10 p-2 text-primary">
        <Icon className="h-4 w-4" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="truncate text-sm font-medium">{title}</span>
          {badge && (
            <Badge variant="secondary" className="shrink-0">
              {badge}
            </Badge>
          )}
          {!enabled && (
            <Badge variant="outline" className="shrink-0">
              disabled
            </Badge>
          )}
        </div>
        {subtitle && (
          <div className="truncate font-mono text-xs text-muted-foreground">
            {subtitle}
          </div>
        )}
      </div>

      {targetable ? (
        <span
          className={cn(
            "flex items-center gap-1 text-xs font-medium",
            attached ? "text-destructive" : "text-primary",
          )}
        >
          {attached ? (
            <>
              <Link2Off className="h-4 w-4" /> Detach
            </>
          ) : (
            <>
              <Link2 className="h-4 w-4" /> Attach
            </>
          )}
        </span>
      ) : (
        <Link
          href={href}
          onClick={(e) => e.stopPropagation()}
          className="rounded-md p-1 text-muted-foreground opacity-0 transition-opacity hover:bg-accent hover:text-foreground group-hover:opacity-100"
          aria-label={`Open ${title}`}
        >
          <ArrowUpRight className="h-4 w-4" />
        </Link>
      )}
    </div>
  );
}
