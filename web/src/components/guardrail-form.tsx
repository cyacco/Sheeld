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
import { GUARD_TYPES, defaultConfig } from "@/components/guard-type-meta";
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

// GuardrailDraft is the shared editing state for the guardrail form and
// the add-guardrail wizard.
export interface GuardrailDraft {
  name: string;
  guardType: string;
  phase: string;
  config: Record<string, unknown>;
  enabled: boolean;
}

export function emptyGuardrailDraft(guardType = "blocklist"): GuardrailDraft {
  return {
    name: "",
    guardType,
    phase: "input",
    config: defaultConfig(guardType),
    enabled: true,
  };
}

export function guardrailDraftFrom(g: Guardrail): GuardrailDraft {
  return {
    name: g.name,
    guardType: g.guard_type,
    phase: g.phase,
    config: g.config,
    enabled: g.enabled,
  };
}

export function guardrailDraftToParams(d: GuardrailDraft): CreateGuardrailParams {
  return {
    name: d.name,
    guard_type: d.guardType,
    phase: d.phase,
    config: d.config,
    enabled: d.enabled,
  };
}

interface FieldGroupProps {
  draft: GuardrailDraft;
  onChange: (draft: GuardrailDraft) => void;
}

// GuardrailBasicsFields: name, phase, enabled. Guard type is chosen by the
// wizard catalog (or immutable on edit), so it isn't part of this group.
export function GuardrailBasicsFields({ draft, onChange }: FieldGroupProps) {
  const set = (patch: Partial<GuardrailDraft>) => onChange({ ...draft, ...patch });
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label>Name</Label>
        <Input
          value={draft.name}
          onChange={(e) => set({ name: e.target.value })}
          required
        />
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label>Phase</Label>
          <Select value={draft.phase} onValueChange={(v) => set({ phase: v })}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="input">Input</SelectItem>
              <SelectItem value="output">Output</SelectItem>
              <SelectItem value="both">Both</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="flex items-end gap-2 pb-2">
          <Switch
            checked={draft.enabled}
            onCheckedChange={(v) => set({ enabled: v })}
          />
          <Label>Enabled</Label>
        </div>
      </div>
    </div>
  );
}

// GuardConfigFields renders the guard-type-specific config component.
export function GuardConfigFields({ draft, onChange }: FieldGroupProps) {
  const setConfig = (config: Record<string, unknown>) =>
    onChange({ ...draft, config });
  return (
    <div className="space-y-2 rounded-md border p-4">
      <h4 className="text-sm font-medium">Guard Configuration</h4>
      {draft.guardType === "blocklist" && (
        <BlocklistConfigForm
          config={draft.config as unknown as BlocklistConfig}
          onChange={(c) => setConfig(c as unknown as Record<string, unknown>)}
        />
      )}
      {draft.guardType === "regex" && (
        <RegexConfigForm
          config={draft.config as unknown as RegexConfig}
          onChange={(c) => setConfig(c as unknown as Record<string, unknown>)}
        />
      )}
      {draft.guardType === "openai_moderation" && (
        <OpenAIModConfigForm
          config={draft.config as unknown as OpenAIModerationConfig}
          onChange={(c) => setConfig(c as unknown as Record<string, unknown>)}
        />
      )}
      {draft.guardType === "guardrails_ai" && (
        <GuardrailsAIConfigForm
          config={draft.config as unknown as GuardrailsAIConfig}
          onChange={(c) => setConfig(c as unknown as Record<string, unknown>)}
        />
      )}
    </div>
  );
}

interface GuardrailFormProps {
  initial?: Guardrail;
  onSubmit: (params: CreateGuardrailParams) => Promise<void>;
  submitLabel: string;
}

// GuardrailForm composes the field groups — used on the guardrail detail
// Configuration tab. Creation goes through the add-guardrail wizard.
export function GuardrailForm({ initial, onSubmit, submitLabel }: GuardrailFormProps) {
  const [draft, setDraft] = useState<GuardrailDraft>(() =>
    initial ? guardrailDraftFrom(initial) : emptyGuardrailDraft(),
  );
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    try {
      await onSubmit(guardrailDraftToParams(draft));
    } finally {
      setLoading(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {!initial && (
        <div className="space-y-2">
          <Label>Guard Type</Label>
          <Select
            value={draft.guardType}
            onValueChange={(v) =>
              setDraft({ ...draft, guardType: v, config: defaultConfig(v) })
            }
          >
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
      )}
      <GuardrailBasicsFields draft={draft} onChange={setDraft} />
      <GuardConfigFields draft={draft} onChange={setDraft} />
      <Button type="submit" disabled={loading}>
        {loading ? "Saving..." : submitLabel}
      </Button>
    </form>
  );
}
