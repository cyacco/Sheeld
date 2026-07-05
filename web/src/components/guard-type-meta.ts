import { Ban, Regex, ShieldCheck, Server, type LucideIcon } from "lucide-react";
import type {
  BlocklistConfig,
  RegexConfig,
  OpenAIModerationConfig,
  GuardrailsAIConfig,
} from "@/lib/types";

export interface GuardTypeMeta {
  value: string;
  label: string;
  description: string;
  icon: LucideIcon;
}

// Drives the wizard catalog tiles and type badges.
export const GUARD_TYPES: GuardTypeMeta[] = [
  {
    value: "blocklist",
    label: "Blocklist",
    description: "Reject text containing any of a list of blocked words.",
    icon: Ban,
  },
  {
    value: "regex",
    label: "Regex",
    description: "Block or require matches against regular expressions.",
    icon: Regex,
  },
  {
    value: "openai_moderation",
    label: "OpenAI Moderation",
    description: "Score content against OpenAI's moderation categories.",
    icon: ShieldCheck,
  },
  {
    value: "guardrails_ai",
    label: "Guardrails AI",
    description: "Validate with a guard hosted on a Guardrails AI server.",
    icon: Server,
  },
];

export function guardTypeMeta(value: string): GuardTypeMeta | undefined {
  return GUARD_TYPES.find((t) => t.value === value);
}

export function defaultConfig(guardType: string): Record<string, unknown> {
  switch (guardType) {
    case "blocklist":
      return { words: [] } satisfies BlocklistConfig;
    case "regex":
      return { patterns: [], mode: "block" } satisfies RegexConfig;
    case "openai_moderation":
      return {
        api_key: "",
        categories: [],
        threshold: 0.5,
        timeout_seconds: 10,
      } satisfies OpenAIModerationConfig;
    case "guardrails_ai":
      return {
        server_url: "",
        guard_name: "",
        timeout_seconds: 10,
        fail_open: false,
      } satisfies GuardrailsAIConfig;
    default:
      return {};
  }
}
