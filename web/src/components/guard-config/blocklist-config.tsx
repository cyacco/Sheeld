"use client";

import { useState, useEffect } from "react";
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
  const [text, setText] = useState(config.words.join("\n"));

  useEffect(() => {
    setText(config.words.join("\n"));
  }, [config.words]);

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label>Words (one per line)</Label>
        <Textarea
          rows={4}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onBlur={() =>
            onChange({
              ...config,
              words: text.split("\n").filter((w) => w.trim()),
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
