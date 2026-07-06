import { Replace, Undo2, UserX, Webhook, type LucideIcon } from "lucide-react";
import type {
  RegexReplaceConfig,
  PresidioConfig,
  WebhookConfig,
} from "@/lib/types";

export interface TransformerTypeMeta {
  value: string;
  label: string;
  description: string;
  icon: LucideIcon;
}

// Drives the wizard catalog tiles and type badges.
export const TRANSFORMER_TYPES: TransformerTypeMeta[] = [
  {
    value: "regex_replace",
    label: "Regex Replace",
    description: "Rewrite message text with pattern → replacement rules.",
    icon: Replace,
  },
  {
    value: "presidio",
    label: "Presidio PII Redaction",
    description: "Detect and redact PII via self-hosted Microsoft Presidio.",
    icon: UserX,
  },
  {
    value: "webhook",
    label: "Webhook",
    description: "Call your own HTTP endpoint to rewrite messages.",
    icon: Webhook,
  },
  {
    value: "deanonymize",
    label: "Deanonymize",
    description:
      "Restore values anonymized by a reversible Presidio step. Attach to the output phase.",
    icon: Undo2,
  },
];

export function transformerTypeMeta(
  value: string,
): TransformerTypeMeta | undefined {
  return TRANSFORMER_TYPES.find((t) => t.value === value);
}

export function defaultTransformerConfig(
  transformerType: string,
): Record<string, unknown> {
  switch (transformerType) {
    case "regex_replace":
      return { rules: [] } satisfies RegexReplaceConfig;
    case "presidio":
      return {
        analyzer_url: "",
        anonymizer_url: "",
        mode: "redact",
        language: "en",
        entities: [],
        timeout_seconds: 10,
      } satisfies PresidioConfig;
    case "deanonymize":
      return {};
    case "webhook":
      return {
        url: "",
        headers: {},
        timeout_seconds: 10,
      } satisfies WebhookConfig;
    default:
      return {};
  }
}
