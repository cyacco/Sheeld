"use client";

import { useState } from "react";
import Link from "next/link";
import { ChevronRight, Trash2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

interface ResourceHeaderProps {
  crumbLabel: string;
  crumbHref: string;
  name: string;
  badges?: string[];
  enabled: boolean;
  onToggleEnabled: (enabled: boolean) => Promise<void>;
  deleteLabel: string;
  onDelete: () => Promise<void>;
}

// Shared detail-page header: breadcrumb, name, badges, enabled switch,
// delete with confirmation.
export function ResourceHeader({
  crumbLabel,
  crumbHref,
  name,
  badges,
  enabled,
  onToggleEnabled,
  deleteLabel,
  onDelete,
}: ResourceHeaderProps) {
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);

  async function handleDelete() {
    setDeleting(true);
    try {
      await onDelete();
    } finally {
      setDeleting(false);
      setConfirmOpen(false);
    }
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-1 text-sm text-muted-foreground">
        <Link href={crumbHref} className="hover:text-foreground">
          {crumbLabel}
        </Link>
        <ChevronRight className="h-4 w-4" />
        <span className="text-foreground">{name}</span>
      </div>
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-bold">{name}</h1>
          {badges?.map((b) => (
            <Badge key={b} variant="secondary">
              {b}
            </Badge>
          ))}
        </div>
        <div className="flex items-center gap-4">
          <div className="flex items-center gap-2">
            <Switch
              id="resource-enabled"
              checked={enabled}
              onCheckedChange={onToggleEnabled}
            />
            <Label htmlFor="resource-enabled">Enabled</Label>
          </div>
          <Button
            variant="outline"
            size="sm"
            className="text-destructive"
            onClick={() => setConfirmOpen(true)}
          >
            <Trash2 className="mr-1 h-4 w-4" /> Delete
          </Button>
        </div>
      </div>

      <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete {name}?</DialogTitle>
            <DialogDescription>{deleteLabel}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmOpen(false)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleDelete} disabled={deleting}>
              {deleting ? "Deleting..." : "Delete"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
