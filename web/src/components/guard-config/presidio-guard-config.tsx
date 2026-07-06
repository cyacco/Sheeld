"use client";

import type { PresidioGuardConfig } from "@/lib/types";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

interface Props {
  config: PresidioGuardConfig;
  onChange: (config: PresidioGuardConfig) => void;
}

export function PresidioGuardConfigForm({ config, onChange }: Props) {
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label>Analyzer URL</Label>
        <Input
          value={config.analyzer_url || ""}
          onChange={(e) => onChange({ ...config, analyzer_url: e.target.value })}
          placeholder="http://presidio-analyzer:3000"
          required
        />
        <p className="text-xs text-muted-foreground">
          Base URL of your self-hosted Presidio analyzer. Requests containing
          detected entities are rejected — use the Presidio transformation
          instead to redact rather than block.
        </p>
      </div>

      <div className="space-y-2">
        <Label>Entities</Label>
        <Input
          value={(config.entities ?? []).join(", ")}
          onChange={(e) =>
            onChange({
              ...config,
              entities: e.target.value
                .split(",")
                .map((s) => s.trim())
                .filter(Boolean),
            })
          }
          placeholder="CREDIT_CARD, US_SSN, EMAIL_ADDRESS"
        />
        <p className="text-xs text-muted-foreground">
          Comma-separated entity types to block on. Leave empty to block on
          any supported entity.
        </p>
      </div>

      <div className="grid grid-cols-3 gap-4">
        <div className="space-y-2">
          <Label>Language</Label>
          <Input
            value={config.language || "en"}
            onChange={(e) => onChange({ ...config, language: e.target.value })}
            placeholder="en"
          />
        </div>
        <div className="space-y-2">
          <Label>Score threshold</Label>
          <Input
            type="number"
            min={0}
            max={1}
            step={0.05}
            value={config.score_threshold ?? 0.5}
            onChange={(e) =>
              onChange({ ...config, score_threshold: Number(e.target.value) })
            }
          />
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
    </div>
  );
}
