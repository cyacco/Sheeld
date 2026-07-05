"use client";

import type { GuardrailsAIConfig } from "@/lib/types";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";

interface Props {
  config: GuardrailsAIConfig;
  onChange: (config: GuardrailsAIConfig) => void;
}

export function GuardrailsAIConfigForm({ config, onChange }: Props) {
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label>Server URL</Label>
        <Input
          value={config.server_url || ""}
          onChange={(e) => onChange({ ...config, server_url: e.target.value })}
          placeholder="http://guardrails:8000"
          required
        />
      </div>

      <div className="space-y-2">
        <Label>Guard Name</Label>
        <Input
          value={config.guard_name || ""}
          onChange={(e) => onChange({ ...config, guard_name: e.target.value })}
          required
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
  );
}
