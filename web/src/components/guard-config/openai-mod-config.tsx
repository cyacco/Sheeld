"use client";

import type { OpenAIModerationConfig } from "@/lib/types";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { Slider } from "@/components/ui/slider";
import { Checkbox } from "@/components/ui/checkbox";

const CATEGORIES = [
  "hate",
  "hate/threatening",
  "harassment",
  "harassment/threatening",
  "self-harm",
  "self-harm/intent",
  "self-harm/instructions",
  "sexual",
  "sexual/minors",
  "violence",
  "violence/graphic",
];

interface Props {
  config: OpenAIModerationConfig;
  onChange: (config: OpenAIModerationConfig) => void;
}

export function OpenAIModConfigForm({ config, onChange }: Props) {
  function toggleCategory(cat: string) {
    const cats = config.categories || [];
    if (cats.includes(cat)) {
      onChange({ ...config, categories: cats.filter((c) => c !== cat) });
    } else {
      onChange({ ...config, categories: [...cats, cat] });
    }
  }

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label>OpenAI API Key</Label>
        <Input
          type="password"
          value={config.api_key || ""}
          onChange={(e) => onChange({ ...config, api_key: e.target.value })}
          required
        />
      </div>

      <div className="space-y-2">
        <Label>Categories (leave unchecked for all)</Label>
        <div className="grid grid-cols-2 gap-2">
          {CATEGORIES.map((cat) => (
            <label key={cat} className="flex items-center gap-2 text-sm">
              <Checkbox
                checked={(config.categories || []).includes(cat)}
                onCheckedChange={() => toggleCategory(cat)}
              />
              {cat}
            </label>
          ))}
        </div>
      </div>

      <div className="space-y-2">
        <Label>Threshold: {config.threshold ?? 0.5}</Label>
        <Slider
          min={0}
          max={1}
          step={0.05}
          value={[config.threshold ?? 0.5]}
          onValueChange={([v]) => onChange({ ...config, threshold: v })}
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
