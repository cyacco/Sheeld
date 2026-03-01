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

interface SourceFormProps {
  initial?: Source;
  onSubmit: (params: CreateSourceParams) => Promise<void>;
  submitLabel: string;
}

export function SourceForm({ initial, onSubmit, submitLabel }: SourceFormProps) {
  const [name, setName] = useState(initial?.name ?? "");
  const [route, setRoute] = useState(initial?.route ?? "");
  const [description, setDescription] = useState(initial?.description ?? "");
  const [llmProvider, setLlmProvider] = useState(initial?.llm_provider ?? "openai");
  const [llmModel, setLlmModel] = useState(initial?.llm_model ?? "gpt-4o");
  const [llmApiKey, setLlmApiKey] = useState("");
  const [passCriteria, setPassCriteria] = useState(initial?.pass_criteria ?? "all");
  const [passThreshold, setPassThreshold] = useState<string>(
    initial?.pass_threshold != null ? String(initial.pass_threshold) : "",
  );
  const [enabled, setEnabled] = useState(initial?.enabled ?? true);
  const [loading, setLoading] = useState(false);
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
    fetchModels(llmProvider);
  }, [llmProvider, fetchModels]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    try {
      const params: CreateSourceParams = {
        name,
        route,
        description: description || undefined,
        llm_provider: llmProvider,
        llm_model: llmModel,
        llm_api_key: llmApiKey,
        pass_criteria: passCriteria,
        pass_threshold: passThreshold ? Number(passThreshold) : undefined,
        enabled,
      };
      await onSubmit(params);
    } finally {
      setLoading(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="name">Name</Label>
          <Input id="name" value={name} onChange={(e) => setName(e.target.value)} required />
        </div>
        <div className="space-y-2">
          <Label htmlFor="route">Route</Label>
          <Input
            id="route"
            value={route}
            onChange={(e) => setRoute(e.target.value)}
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
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          rows={2}
        />
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="llm_provider">LLM Provider</Label>
          <Select value={llmProvider} onValueChange={setLlmProvider}>
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
              onClick={() => fetchModels(llmProvider)}
              disabled={modelsLoading}
              className="inline-flex items-center justify-center rounded-md p-1 text-muted-foreground hover:text-foreground hover:bg-accent disabled:opacity-50"
              title="Refresh models"
            >
              <RefreshCw className={`h-3.5 w-3.5 ${modelsLoading ? "animate-spin" : ""}`} />
            </button>
          </div>
          <Select value={llmModel} onValueChange={setLlmModel}>
            <SelectTrigger>
              <SelectValue placeholder={modelsLoading ? "Loading..." : "Select a model"} />
            </SelectTrigger>
            <SelectContent>
              {/* Keep current value as option if not in fetched list */}
              {llmModel && !models.some((m) => m.id === llmModel) && (
                <SelectItem value={llmModel}>{llmModel}</SelectItem>
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
          value={llmApiKey}
          onChange={(e) => setLlmApiKey(e.target.value)}
          placeholder={initial ? "(unchanged)" : ""}
          required={!initial}
        />
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="pass_criteria">Pass Criteria</Label>
          <Select value={passCriteria} onValueChange={setPassCriteria}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All guards must pass</SelectItem>
              <SelectItem value="any">Any guard must pass</SelectItem>
              <SelectItem value="threshold">Threshold</SelectItem>
            </SelectContent>
          </Select>
        </div>
        {passCriteria === "threshold" && (
          <div className="space-y-2">
            <Label htmlFor="pass_threshold">Threshold</Label>
            <Input
              id="pass_threshold"
              type="number"
              min={1}
              value={passThreshold}
              onChange={(e) => setPassThreshold(e.target.value)}
            />
          </div>
        )}
      </div>

      <div className="flex items-center gap-2">
        <Switch id="enabled" checked={enabled} onCheckedChange={setEnabled} />
        <Label htmlFor="enabled">Enabled</Label>
      </div>

      <Button type="submit" disabled={loading}>
        {loading ? "Saving..." : submitLabel}
      </Button>
    </form>
  );
}
