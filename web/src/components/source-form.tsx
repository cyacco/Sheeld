"use client";

import { useState, useEffect, useCallback } from "react";
import { RefreshCw } from "lucide-react";
import type { CreateSourceParams, Source, ModelInfo } from "@/lib/types";
import { listModels } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

// SourceDraft is the shared editing state for source forms and the
// add-source wizard.
export interface SourceDraft {
  name: string;
  route: string;
  description: string;
  llmProvider: string;
  llmModel: string;
  llmApiKey: string;
  llmBaseUrl: string;
  inputPassCriteria: string;
  inputPassThreshold: string;
  outputPassCriteria: string;
  outputPassThreshold: string;
  enabled: boolean;
}

export function emptySourceDraft(): SourceDraft {
  return {
    name: "",
    route: "",
    description: "",
    llmProvider: "openai",
    llmModel: "gpt-4o",
    llmApiKey: "",
    llmBaseUrl: "",
    inputPassCriteria: "all",
    inputPassThreshold: "",
    outputPassCriteria: "all",
    outputPassThreshold: "",
    enabled: true,
  };
}

export function sourceDraftFrom(source: Source): SourceDraft {
  return {
    name: source.name,
    route: source.route,
    description: source.description ?? "",
    llmProvider: source.llm_provider,
    llmModel: source.llm_model,
    llmApiKey: "",
    llmBaseUrl: source.llm_base_url ?? "",
    inputPassCriteria: source.input_pass_criteria,
    inputPassThreshold:
      source.input_pass_threshold != null ? String(source.input_pass_threshold) : "",
    outputPassCriteria: source.output_pass_criteria,
    outputPassThreshold:
      source.output_pass_threshold != null ? String(source.output_pass_threshold) : "",
    enabled: source.enabled,
  };
}

export function sourceDraftToParams(draft: SourceDraft): CreateSourceParams {
  return {
    name: draft.name,
    route: draft.route,
    description: draft.description || undefined,
    llm_provider: draft.llmProvider,
    llm_model: draft.llmModel,
    llm_api_key: draft.llmApiKey,
    llm_base_url: draft.llmBaseUrl || undefined,
    input_pass_criteria: draft.inputPassCriteria,
    input_pass_threshold: draft.inputPassThreshold
      ? Number(draft.inputPassThreshold)
      : undefined,
    output_pass_criteria: draft.outputPassCriteria,
    output_pass_threshold: draft.outputPassThreshold
      ? Number(draft.outputPassThreshold)
      : undefined,
    enabled: draft.enabled,
  };
}

interface FieldGroupProps {
  draft: SourceDraft;
  onChange: (draft: SourceDraft) => void;
}

// SourceBasicsFields: name, route, description.
export function SourceBasicsFields({ draft, onChange }: FieldGroupProps) {
  const set = (patch: Partial<SourceDraft>) => onChange({ ...draft, ...patch });
  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="name">Name</Label>
          <Input
            id="name"
            value={draft.name}
            onChange={(e) => set({ name: e.target.value })}
            required
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="route">Route</Label>
          <Input
            id="route"
            value={draft.route}
            onChange={(e) => set({ route: e.target.value })}
            placeholder="my-source"
            required
          />
          <p className="text-xs text-muted-foreground">
            Used in your proxy URL: /v1/proxy/your-route
          </p>
        </div>
      </div>

      <div className="space-y-2">
        <Label htmlFor="description">Description</Label>
        <Textarea
          id="description"
          value={draft.description}
          onChange={(e) => set({ description: e.target.value })}
          rows={2}
        />
      </div>
    </div>
  );
}

// SourceLLMFields: provider, model (fetched per provider), API key, pass
// criteria/threshold, enabled.
export function SourceLLMFields({
  draft,
  onChange,
  isUpdate,
}: FieldGroupProps & { isUpdate?: boolean }) {
  const set = (patch: Partial<SourceDraft>) => onChange({ ...draft, ...patch });
  const [models, setModels] = useState<ModelInfo[]>([]);
  const [modelsLoading, setModelsLoading] = useState(false);

  const fetchModels = useCallback(async (provider: string) => {
    setModelsLoading(true);
    try {
      const result = await listModels(provider);
      setModels(result);
    } catch {
      setModels([]);
    } finally {
      setModelsLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchModels(draft.llmProvider);
  }, [draft.llmProvider, fetchModels]);

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="llm_provider">LLM Provider</Label>
          <Select
            value={draft.llmProvider}
            onValueChange={(v) => set({ llmProvider: v })}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="openai">OpenAI</SelectItem>
              <SelectItem value="anthropic">Anthropic</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <div className="flex items-center gap-1">
            <Label htmlFor="llm_model">Model</Label>
            <button
              type="button"
              onClick={() => fetchModels(draft.llmProvider)}
              disabled={modelsLoading}
              className="inline-flex items-center justify-center rounded-md p-1 text-muted-foreground hover:text-foreground hover:bg-accent disabled:opacity-50"
              title="Refresh models"
            >
              <RefreshCw
                className={`h-3.5 w-3.5 ${modelsLoading ? "animate-spin" : ""}`}
              />
            </button>
          </div>
          <Select
            value={draft.llmModel}
            onValueChange={(v) => set({ llmModel: v })}
          >
            <SelectTrigger>
              <SelectValue
                placeholder={modelsLoading ? "Loading..." : "Select a model"}
              />
            </SelectTrigger>
            <SelectContent>
              {/* Keep current value as option if not in fetched list */}
              {draft.llmModel && !models.some((m) => m.id === draft.llmModel) && (
                <SelectItem value={draft.llmModel}>{draft.llmModel}</SelectItem>
              )}
              {models.map((m) => (
                <SelectItem key={m.id} value={m.id}>
                  {m.id}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      <div className="space-y-2">
        <Label htmlFor="llm_api_key">LLM API Key</Label>
        <Input
          id="llm_api_key"
          type="password"
          value={draft.llmApiKey}
          onChange={(e) => set({ llmApiKey: e.target.value })}
          placeholder={isUpdate ? "(unchanged)" : ""}
          required={!isUpdate}
        />
      </div>

      <div className="space-y-2">
        <Label htmlFor="llm_base_url">LLM Base URL (optional)</Label>
        <Input
          id="llm_base_url"
          type="url"
          value={draft.llmBaseUrl}
          onChange={(e) => set({ llmBaseUrl: e.target.value })}
          placeholder="https://api.openai.com/v1"
        />
        <p className="text-xs text-muted-foreground">
          Any OpenAI-compatible endpoint. Leave empty to use the data
          plane&apos;s configured gateway.
        </p>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="input_pass_criteria">Input Pass Criteria</Label>
          <Select
            value={draft.inputPassCriteria}
            onValueChange={(v) => set({ inputPassCriteria: v })}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All guards must pass</SelectItem>
              <SelectItem value="any">Any guard must pass</SelectItem>
              <SelectItem value="n_of_m">At least N guards pass</SelectItem>
            </SelectContent>
          </Select>
        </div>
        {draft.inputPassCriteria === "n_of_m" && (
          <div className="space-y-2">
            <Label htmlFor="input_pass_threshold">Input Threshold (N)</Label>
            <Input
              id="input_pass_threshold"
              type="number"
              min={1}
              value={draft.inputPassThreshold}
              onChange={(e) => set({ inputPassThreshold: e.target.value })}
            />
          </div>
        )}
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="output_pass_criteria">Output Pass Criteria</Label>
          <Select
            value={draft.outputPassCriteria}
            onValueChange={(v) => set({ outputPassCriteria: v })}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All guards must pass</SelectItem>
              <SelectItem value="any">Any guard must pass</SelectItem>
              <SelectItem value="n_of_m">At least N guards pass</SelectItem>
            </SelectContent>
          </Select>
        </div>
        {draft.outputPassCriteria === "n_of_m" && (
          <div className="space-y-2">
            <Label htmlFor="output_pass_threshold">Output Threshold (N)</Label>
            <Input
              id="output_pass_threshold"
              type="number"
              min={1}
              value={draft.outputPassThreshold}
              onChange={(e) => set({ outputPassThreshold: e.target.value })}
            />
          </div>
        )}
      </div>

      <div className="flex items-center gap-2">
        <Switch
          id="enabled"
          checked={draft.enabled}
          onCheckedChange={(v) => set({ enabled: v })}
        />
        <Label htmlFor="enabled">Enabled</Label>
      </div>
    </div>
  );
}

interface SourceFormProps {
  initial?: Source;
  onSubmit: (params: CreateSourceParams) => Promise<void>;
  submitLabel: string;
}

// SourceForm composes both field groups — used on the source detail
// Configuration tab. Creation goes through the add-source wizard.
export function SourceForm({ initial, onSubmit, submitLabel }: SourceFormProps) {
  const [draft, setDraft] = useState<SourceDraft>(() =>
    initial ? sourceDraftFrom(initial) : emptySourceDraft(),
  );
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    try {
      await onSubmit(sourceDraftToParams(draft));
    } finally {
      setLoading(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <SourceBasicsFields draft={draft} onChange={setDraft} />
      <SourceLLMFields draft={draft} onChange={setDraft} isUpdate={!!initial} />
      <Button type="submit" disabled={loading}>
        {loading ? "Saving..." : submitLabel}
      </Button>
    </form>
  );
}
