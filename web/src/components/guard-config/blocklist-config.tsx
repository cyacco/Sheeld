"use client";

import type { BlocklistConfig } from "@/lib/types";
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
  config: BlocklistConfig;
  onChange: (config: BlocklistConfig) => void;
}

export function BlocklistConfigForm({ config, onChange }: Props) {
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label>Words (one per line)</Label>
        <Textarea
          rows={4}
          value={config.words.join("\n")}
          onChange={(e) =>
            onChange({
              ...config,
              words: e.target.value.split("\n").filter((w) => w.trim()),
            })
          }
        />
      </div>
      <div className="space-y-2">
        <Label>Mode</Label>
        <Select value={config.mode || "block"} onValueChange={(v) => onChange({ ...config, mode: v })}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="block">Block (reject if word found)</SelectItem>
            <SelectItem value="allow">Allow (reject if word NOT found)</SelectItem>
          </SelectContent>
        </Select>
      </div>
    </div>
  );
}
