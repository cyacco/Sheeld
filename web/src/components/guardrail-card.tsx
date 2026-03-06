"use client";

import Link from "next/link";
import type { Guardrail } from "@/lib/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

interface Props {
  guardrail: Guardrail;
  onToggle: (guardrail: Guardrail, enabled: boolean) => void;
  onEdit: (guardrail: Guardrail) => void;
  onDelete: (guardrail: Guardrail) => void;
  href?: string;
  deleteLabel?: string;
}

export function GuardrailCard({ guardrail, onToggle, onEdit, onDelete, href, deleteLabel = "Remove" }: Props) {
  const title = href ? (
    <Link href={href} className="hover:underline">
      {guardrail.name}
    </Link>
  ) : (
    guardrail.name
  );

  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between">
          <CardTitle className="text-base">{title}</CardTitle>
          <div className="flex items-center gap-2">
            <Badge variant="outline">{guardrail.guard_type}</Badge>
            <Badge variant="secondary">{guardrail.phase}</Badge>
          </div>
        </div>
      </CardHeader>
      <CardContent>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Switch
              checked={guardrail.enabled}
              onCheckedChange={(v) => onToggle(guardrail, v)}
            />
            <span className="text-sm text-muted-foreground">
              {guardrail.enabled ? "Enabled" : "Disabled"}
            </span>
          </div>
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={() => onEdit(guardrail)}>
              Edit
            </Button>
            <Button variant="destructive" size="sm" onClick={() => onDelete(guardrail)}>
              {deleteLabel}
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
