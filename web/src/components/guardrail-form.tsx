"use client";

import { useState } from "react";
import type {
  CreateGuardrailParams,
  Guardrail,
  BlocklistConfig,
  RegexConfig,
  OpenAIModerationConfig,
  GuardrailsAIConfig,
} from "@/lib/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { BlocklistConfigForm } from "@/components/guard-config/blocklist-config";
import { RegexConfigForm } from "@/components/guard-config/regex-config";
import { OpenAIModConfigForm } from "@/components/guard-config/openai-mod-config";
import { GuardrailsAIConfigForm } from "@/components/guard-config/guardrails-ai-config";

const GUARD_TYPES = [
  { value: "blocklist", label: "Blocklist" },
  { value: "regex", label: "Regex" },
  { value: "openai_moderation", label: "OpenAI Moderation" },
  { value: "guardrails_ai", label: "Guardrails AI" },
];

function defaultConfig(guardType: string): Record<string, unknown> {
  switch (guardType) {
    case "blocklist":
      return { words: [] } satisfies BlocklistConfig;
    case "regex":
      return { patterns: [], mode: "block" } satisfies RegexConfig;
    case "openai_moderation":
      return { api_key: "", categories: [], threshold: 0.5, timeout_seconds: 10 } satisfies OpenAIModerationConfig;
    case "guardrails_ai":
      return { server_url: "", guard_name: "", timeout_seconds: 10, fail_open: false } satisfies GuardrailsAIConfig;
    default:
      return {};
  }
}

interface GuardrailFormProps {
  initial?: Guardrail;
  onSubmit: (params: CreateGuardrailParams) => Promise<void>;
  submitLabel: string;
}

export function GuardrailForm({ initial, onSubmit, submitLabel }: GuardrailFormProps) {
  const [name, setName] = useState(initial?.name ?? "");
  const [guardType, setGuardType] = useState(initial?.guard_type ?? "blocklist");
  const [phase, setPhase] = useState(initial?.phase ?? "input");
  const [config, setConfig] = useState<Record<string, unknown>>(
    initial?.config ?? defaultConfig("blocklist"),
  );
  const [enabled, setEnabled] = useState(initial?.enabled ?? true);
  const [loading, setLoading] = useState(false);

  function handleGuardTypeChange(newType: string) {
    setGuardType(newType);
    setConfig(defaultConfig(newType));
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    try {
      await onSubmit({
        name,
        guard_type: guardType,
        phase,
        config,
        enabled,
      });
    } finally {
      setLoading(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="space-y-2">
        <Label>Name</Label>
        <Input value={name} onChange={(e) => setName(e.target.value)} required />
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label>Guard Type</Label>
          <Select value={guardType} onValueChange={handleGuardTypeChange} disabled={!!initial}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {GUARD_TYPES.map((gt) => (
                <SelectItem key={gt.value} value={gt.value}>
                  {gt.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <Label>Phase</Label>
          <Select value={phase} onValueChange={setPhase}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="input">Input</SelectItem>
              <SelectItem value="output">Output</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      <div className="border rounded-md p-4 space-y-2">
        <h4 className="font-medium text-sm">Guard Configuration</h4>
        {guardType === "blocklist" && (
          <BlocklistConfigForm
            config={config as unknown as BlocklistConfig}
            onChange={(c) => setConfig(c as unknown as Record<string, unknown>)}
          />
        )}
        {guardType === "regex" && (
          <RegexConfigForm
            config={config as unknown as RegexConfig}
            onChange={(c) => setConfig(c as unknown as Record<string, unknown>)}
          />
        )}
        {guardType === "openai_moderation" && (
          <OpenAIModConfigForm
            config={config as unknown as OpenAIModerationConfig}
            onChange={(c) => setConfig(c as unknown as Record<string, unknown>)}
          />
        )}
        {guardType === "guardrails_ai" && (
          <GuardrailsAIConfigForm
            config={config as unknown as GuardrailsAIConfig}
            onChange={(c) => setConfig(c as unknown as Record<string, unknown>)}
          />
        )}
      </div>

      <div className="flex items-center gap-2">
        <Switch checked={enabled} onCheckedChange={setEnabled} />
        <Label>Enabled</Label>
      </div>

      <Button type="submit" disabled={loading}>
        {loading ? "Saving..." : submitLabel}
      </Button>
    </form>
  );
}
