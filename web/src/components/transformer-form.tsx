"use client";

import { useState } from "react";
import type {
  CreateTransformerParams,
  Transformer,
  RegexReplaceConfig,
  PresidioConfig,
  WebhookConfig,
} from "@/lib/types";
import { defaultTransformerConfig } from "@/components/transformer-type-meta";
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
import { RegexReplaceConfigForm } from "@/components/transformer-config/regex-replace-config";
import { PresidioConfigForm } from "@/components/transformer-config/presidio-config";
import { TransformerWebhookConfigForm } from "@/components/transformer-config/transformer-webhook-config";

// TransformerDraft is the shared editing state for the transformer form and
// the add-transformation wizard.
export interface TransformerDraft {
  name: string;
  transformerType: string;
  phase: string;
  config: Record<string, unknown>;
  enabled: boolean;
}

export function emptyTransformerDraft(
  transformerType = "regex_replace",
): TransformerDraft {
  return {
    name: "",
    transformerType,
    phase: "input",
    config: defaultTransformerConfig(transformerType),
    enabled: true,
  };
}

export function transformerDraftFrom(t: Transformer): TransformerDraft {
  return {
    name: t.name,
    transformerType: t.transformer_type,
    phase: t.phase,
    config: t.config,
    enabled: t.enabled,
  };
}

export function transformerDraftToParams(
  d: TransformerDraft,
): CreateTransformerParams {
  return {
    name: d.name,
    transformer_type: d.transformerType,
    phase: d.phase,
    config: d.config,
    enabled: d.enabled,
  };
}

interface FieldGroupProps {
  draft: TransformerDraft;
  onChange: (draft: TransformerDraft) => void;
}

// TransformerBasicsFields: name, phase, enabled. Transformer type is chosen
// by the wizard catalog (or immutable on edit).
export function TransformerBasicsFields({ draft, onChange }: FieldGroupProps) {
  const set = (patch: Partial<TransformerDraft>) =>
    onChange({ ...draft, ...patch });
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
              <SelectItem value="input">Input (rewrites the request)</SelectItem>
              <SelectItem value="output">Output (rewrites the response)</SelectItem>
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

// TransformerConfigFields renders the type-specific config component plus
// the shared on_error policy selector.
export function TransformerConfigFields({ draft, onChange }: FieldGroupProps) {
  const setConfig = (config: Record<string, unknown>) =>
    onChange({ ...draft, config });
  return (
    <div className="space-y-2 rounded-md border p-4">
      <h4 className="text-sm font-medium">Transformation Configuration</h4>
      {draft.transformerType === "regex_replace" && (
        <RegexReplaceConfigForm
          config={draft.config as unknown as RegexReplaceConfig}
          onChange={(c) => setConfig(c as unknown as Record<string, unknown>)}
        />
      )}
      {draft.transformerType === "presidio" && (
        <PresidioConfigForm
          config={draft.config as unknown as PresidioConfig}
          onChange={(c) => setConfig(c as unknown as Record<string, unknown>)}
        />
      )}
      {draft.transformerType === "webhook" && (
        <TransformerWebhookConfigForm
          config={draft.config as unknown as WebhookConfig}
          onChange={(c) => setConfig(c as unknown as Record<string, unknown>)}
        />
      )}

      <div className="space-y-2 pt-2">
        <Label>On error</Label>
        <Select
          value={(draft.config.on_error as string) ?? "fail_closed"}
          onValueChange={(v) => setConfig({ ...draft.config, on_error: v })}
        >
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="fail_closed">Fail closed (block request)</SelectItem>
            <SelectItem value="fail_open">Fail open (skip this step)</SelectItem>
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground">
          What happens when this transformation errors (e.g. its service is
          unreachable). Fail closed blocks the request rather than forwarding
          untransformed text.
        </p>
      </div>
    </div>
  );
}

interface TransformerFormProps {
  initial?: Transformer;
  onSubmit: (params: CreateTransformerParams) => Promise<void>;
  submitLabel: string;
}

// TransformerForm composes the field groups — used on the transformation
// detail Configuration tab. Creation goes through the wizard.
export function TransformerForm({ initial, onSubmit, submitLabel }: TransformerFormProps) {
  const [draft, setDraft] = useState<TransformerDraft>(() =>
    initial ? transformerDraftFrom(initial) : emptyTransformerDraft(),
  );
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    try {
      await onSubmit(transformerDraftToParams(draft));
    } finally {
      setLoading(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <TransformerBasicsFields draft={draft} onChange={setDraft} />
      <TransformerConfigFields draft={draft} onChange={setDraft} />
      <Button type="submit" disabled={loading}>
        {loading ? "Saving..." : submitLabel}
      </Button>
    </form>
  );
}
