import { Ban, BrainCircuit, Regex, ShieldCheck, Server, UserX, Webhook, type LucideIcon } from "lucide-react";
import type {
  BlocklistConfig,
  RegexConfig,
  OpenAIModerationConfig,
  GuardrailsAIConfig,
  WebhookConfig,
  LLMClassifierConfig,
  PresidioGuardConfig,
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
  {
    value: "webhook",
    label: "Webhook",
    description: "Call your own HTTP endpoint to validate text.",
    icon: Webhook,
  },
  {
    value: "llm_classifier",
    label: "LLM Classifier",
    description: "Ask a small LLM whether text violates a plain-language policy.",
    icon: BrainCircuit,
  },
  {
    value: "presidio",
    label: "Presidio PII Detection",
    description: "Reject text containing PII detected by self-hosted Presidio.",
    icon: UserX,
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
      } satisfies GuardrailsAIConfig;
    case "webhook":
      return {
        url: "",
        headers: {},
        timeout_seconds: 10,
      } satisfies WebhookConfig;
    case "llm_classifier":
      return {
        base_url: "https://api.openai.com/v1",
        api_key: "",
        model: "gpt-4o-mini",
        instructions: "",
        timeout_seconds: 15,
      } satisfies LLMClassifierConfig;
    case "presidio":
      return {
        analyzer_url: "",
        language: "en",
        entities: [],
        score_threshold: 0.5,
        timeout_seconds: 10,
      } satisfies PresidioGuardConfig;
    default:
      return {};
  }
}
