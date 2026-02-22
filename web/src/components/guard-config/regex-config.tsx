"use client";

import type { RegexConfig } from "@/lib/types";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

interface Props {
  config: RegexConfig;
  onChange: (config: RegexConfig) => void;
}

export function RegexConfigForm({ config, onChange }: Props) {
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label>Patterns (one per line)</Label>
        <Textarea
          rows={4}
          value={config.patterns.join("\n")}
          onChange={(e) =>
            onChange({
              ...config,
              patterns: e.target.value.split("\n").filter((p) => p.trim()),
            })
          }
          placeholder="e.g. \b(SSN|social security)\b"
        />
      </div>
      <div className="space-y-2">
        <Label>Mode</Label>
        <Select value={config.mode || "block"} onValueChange={(v) => onChange({ ...config, mode: v })}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="block">Block (reject if pattern matches)</SelectItem>
            <SelectItem value="require">Require (reject if pattern doesn&apos;t match)</SelectItem>
          </SelectContent>
        </Select>
      </div>
    </div>
  );
}
