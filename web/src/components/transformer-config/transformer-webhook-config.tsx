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

// Same shape as the webhook guard form but documents the transformer
// contract: messages in, rewritten messages out.
export function TransformerWebhookConfigForm({ config, onChange }: Props) {
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
          placeholder="https://transforms.internal/rewrite"
          required
        />
        <p className="text-xs text-muted-foreground">
          Receives POST {"{"}&quot;messages&quot;, &quot;phase&quot;,
          &quot;source_route&quot;{"}"} and must return {"{"}
          &quot;messages&quot;: [...]{"}"} — the full rewritten array.
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
