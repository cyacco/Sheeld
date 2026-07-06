"use client";

import { Plus, X } from "lucide-react";
import type { RegexReplaceConfig, RegexReplaceRule } from "@/lib/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

interface Props {
  config: RegexReplaceConfig;
  onChange: (config: RegexReplaceConfig) => void;
}

export function RegexReplaceConfigForm({ config, onChange }: Props) {
  const rules = config.rules ?? [];

  function setRules(next: RegexReplaceRule[]) {
    onChange({ ...config, rules: next });
  }

  return (
    <div className="space-y-2">
      <Label>Rules</Label>
      <p className="text-xs text-muted-foreground">
        Applied in order to every message. Go (RE2) regex syntax; $1-style
        group references work in the replacement.
      </p>
      {rules.map((rule, i) => (
        <div key={i} className="flex items-center gap-2">
          <Input
            value={rule.pattern}
            onChange={(e) =>
              setRules(rules.map((r, j) => (j === i ? { ...r, pattern: e.target.value } : r)))
            }
            placeholder={"\\b\\d{3}-\\d{2}-\\d{4}\\b"}
            className="font-mono"
          />
          <Input
            value={rule.replace}
            onChange={(e) =>
              setRules(rules.map((r, j) => (j === i ? { ...r, replace: e.target.value } : r)))
            }
            placeholder="[SSN]"
            className="w-1/3 font-mono"
          />
          <Button
            type="button"
            variant="ghost"
            size="icon"
            onClick={() => setRules(rules.filter((_, j) => j !== i))}
            aria-label="Remove rule"
          >
            <X className="h-4 w-4" />
          </Button>
        </div>
      ))}
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={() => setRules([...rules, { pattern: "", replace: "" }])}
      >
        <Plus className="mr-1 h-4 w-4" /> Add rule
      </Button>
    </div>
  );
}
