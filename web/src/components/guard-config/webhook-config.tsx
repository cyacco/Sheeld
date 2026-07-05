"use client";

import { useState } from "react";
import { Plus, X } from "lucide-react";
import type { WebhookConfig } from "@/lib/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

interface Props {
  config: WebhookConfig;
  onChange: (config: WebhookConfig) => void;
}

interface HeaderRow {
  key: string;
  value: string;
}

export function WebhookConfigForm({ config, onChange }: Props) {
  // Header rows are local state so partially-typed rows (empty key or
  // value) survive edits; only complete rows are emitted to the config.
  const [rows, setRows] = useState<HeaderRow[]>(() =>
    Object.entries(config.headers ?? {}).map(([key, value]) => ({ key, value })),
  );

  function emitRows(next: HeaderRow[]) {
    setRows(next);
    const headers: Record<string, string> = {};
    for (const r of next) {
      if (r.key.trim() !== "") headers[r.key.trim()] = r.value;
    }
    onChange({ ...config, headers });
  }

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label>Endpoint URL</Label>
        <Input
          value={config.url || ""}
          onChange={(e) => onChange({ ...config, url: e.target.value })}
          placeholder="https://guards.internal/validate"
          required
        />
        <p className="text-xs text-muted-foreground">
          Receives POST {"{"}&quot;input&quot;, &quot;phase&quot;,
          &quot;source_route&quot;{"}"} and must return {"{"}&quot;passed&quot;:
          bool, &quot;message&quot;?, &quot;details&quot;?{"}"}.
        </p>
      </div>

      <div className="space-y-2">
        <Label>Headers</Label>
        {rows.map((row, i) => (
          <div key={i} className="flex items-center gap-2">
            <Input
              value={row.key}
              onChange={(e) =>
                emitRows(rows.map((r, j) => (j === i ? { ...r, key: e.target.value } : r)))
              }
              placeholder="Authorization"
              className="w-1/3"
            />
            <Input
              value={row.value}
              onChange={(e) =>
                emitRows(rows.map((r, j) => (j === i ? { ...r, value: e.target.value } : r)))
              }
              placeholder="Bearer ..."
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              onClick={() => emitRows(rows.filter((_, j) => j !== i))}
              aria-label="Remove header"
            >
              <X className="h-4 w-4" />
            </Button>
          </div>
        ))}
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => setRows([...rows, { key: "", value: "" }])}
        >
          <Plus className="mr-1 h-4 w-4" /> Add header
        </Button>
      </div>

      <div className="space-y-2">
        <Label>Timeout (seconds)</Label>
        <Input
          type="number"
          min={1}
          value={config.timeout_seconds || 10}
          onChange={(e) =>
            onChange({ ...config, timeout_seconds: Number(e.target.value) })
          }
        />
      </div>
    </div>
  );
}
