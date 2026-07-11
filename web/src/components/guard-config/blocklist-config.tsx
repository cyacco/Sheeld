"use client";

import { useState } from "react";
import type { BlocklistConfig } from "@/lib/types";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";

interface Props {
  config: BlocklistConfig;
  onChange: (config: BlocklistConfig) => void;
}

export function BlocklistConfigForm({ config, onChange }: Props) {
  const [text, setText] = useState(config.words.join("\n"));

  // Re-sync the local textarea when the incoming words change (e.g. switching
  // guards). Adjusting state during render — comparing against the previous
  // prop — avoids a setState-in-effect while keeping the same trigger.
  const [prevWords, setPrevWords] = useState(config.words);
  if (config.words !== prevWords) {
    setPrevWords(config.words);
    setText(config.words.join("\n"));
  }

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label>Words (one per line)</Label>
        <Textarea
          rows={4}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onBlur={() => {
            const seen = new Set<string>();
            const unique = text
              .split("\n")
              .filter((w) => w.trim())
              .filter((w) => {
                const key = w.trim().toLowerCase();
                if (seen.has(key)) return false;
                seen.add(key);
                return true;
              });
            setText(unique.join("\n"));
            onChange({ ...config, words: unique });
          }}
        />
      </div>
    </div>
  );
}
