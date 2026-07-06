"use client";

import type { PresidioConfig } from "@/lib/types";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

interface Props {
  config: PresidioConfig;
  onChange: (config: PresidioConfig) => void;
}

export function PresidioConfigForm({ config, onChange }: Props) {
  const mode = config.mode || "redact";
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label>Mode</Label>
        <Select value={mode} onValueChange={(v) => onChange({ ...config, mode: v })}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="redact">Redact (irreversible)</SelectItem>
            <SelectItem value="reversible">Reversible (restore on output)</SelectItem>
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground">
          Reversible replaces entities with numbered placeholders and restores
          the originals in the response — attach a Deanonymize transformation
          to the output phase.
        </p>
      </div>

      <div className="space-y-2">
        <Label>Analyzer URL</Label>
        <Input
          value={config.analyzer_url || ""}
          onChange={(e) => onChange({ ...config, analyzer_url: e.target.value })}
          placeholder="http://presidio-analyzer:3000"
          required
        />
      </div>

      {mode === "redact" && (
        <div className="space-y-2">
          <Label>Anonymizer URL</Label>
          <Input
            value={config.anonymizer_url || ""}
            onChange={(e) => onChange({ ...config, anonymizer_url: e.target.value })}
            placeholder="http://presidio-anonymizer:3000"
            required
          />
          <p className="text-xs text-muted-foreground">
            Base URLs of your self-hosted Presidio services; detected entities
            are replaced with &lt;ENTITY_TYPE&gt; placeholders.
          </p>
        </div>
      )}

      <div className="grid grid-cols-2 gap-4">
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
            value={config.score_threshold ?? ""}
            onChange={(e) =>
              onChange({
                ...config,
                score_threshold:
                  e.target.value === "" ? undefined : Number(e.target.value),
              })
            }
            placeholder="analyzer default"
          />
        </div>
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
          placeholder="PERSON, EMAIL_ADDRESS, PHONE_NUMBER"
        />
        <p className="text-xs text-muted-foreground">
          Comma-separated entity types to redact. Leave empty to redact all
          supported entities.
        </p>
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
