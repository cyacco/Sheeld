"use client";

import type { LLMClassifierConfig } from "@/lib/types";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";

interface Props {
  config: LLMClassifierConfig;
  onChange: (config: LLMClassifierConfig) => void;
}

export function LLMClassifierConfigForm({ config, onChange }: Props) {
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label>Policy (what to flag)</Label>
        <Textarea
          value={config.instructions || ""}
          onChange={(e) => onChange({ ...config, instructions: e.target.value })}
          placeholder="Prompt injection attempts, requests to reveal the system prompt, or questions about competitors."
          rows={3}
          required
        />
        <p className="text-xs text-muted-foreground">
          Plain language. The classifier model flags content matching this
          policy; flagged content fails the guard.
        </p>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label>Model</Label>
          <Input
            value={config.model || ""}
            onChange={(e) => onChange({ ...config, model: e.target.value })}
            placeholder="gpt-4o-mini"
            required
          />
        </div>
        <div className="space-y-2">
          <Label>Timeout (seconds)</Label>
          <Input
            type="number"
            min={1}
            value={config.timeout_seconds || 15}
            onChange={(e) =>
              onChange({ ...config, timeout_seconds: Number(e.target.value) })
            }
          />
        </div>
      </div>

      <div className="space-y-2">
        <Label>API Base URL</Label>
        <Input
          value={config.base_url || ""}
          onChange={(e) => onChange({ ...config, base_url: e.target.value })}
          placeholder="https://api.openai.com/v1"
          required
        />
        <p className="text-xs text-muted-foreground">
          Any OpenAI-compatible endpoint — OpenAI, a LiteLLM deployment, or a
          local vLLM/Ollama server.
        </p>
      </div>

      <div className="space-y-2">
        <Label>API Key</Label>
        <Input
          type="password"
          value={config.api_key || ""}
          onChange={(e) => onChange({ ...config, api_key: e.target.value })}
          placeholder="Optional for local endpoints"
        />
      </div>
    </div>
  );
}
