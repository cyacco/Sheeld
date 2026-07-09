export interface Organization {
  id: string;
  name: string;
  created_at: string;
  updated_at: string;
}

export interface User {
  id: string;
  organization_id: string;
  email: string;
  created_at: string;
  updated_at: string;
}

export interface RegisterResult {
  organization: Organization;
  user: User;
  token: string;
}

export interface LoginResult {
  user: User;
  token: string;
}

export interface Source {
  id: string;
  name: string;
  route: string;
  description?: string;
  llm_provider: string;
  llm_model: string;
  llm_base_url?: string;
  input_pass_criteria: string;
  input_pass_threshold?: number;
  output_pass_criteria: string;
  output_pass_threshold?: number;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateSourceParams {
  name: string;
  route: string;
  description?: string;
  llm_provider: string;
  llm_model: string;
  llm_api_key: string;
  llm_base_url?: string;
  input_pass_criteria: string;
  input_pass_threshold?: number;
  output_pass_criteria: string;
  output_pass_threshold?: number;
  enabled: boolean;
}

export type UpdateSourceParams = CreateSourceParams;

export interface Guardrail {
  id: string;
  organization_id: string;
  name: string;
  guard_type: string;
  phase: string;
  config: Record<string, unknown>;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateGuardrailParams {
  name: string;
  guard_type: string;
  phase: string;
  config: Record<string, unknown>;
  enabled: boolean;
}

export type UpdateGuardrailParams = CreateGuardrailParams;

export interface Transformer {
  id: string;
  organization_id: string;
  name: string;
  transformer_type: string;
  phase: string;
  config: Record<string, unknown>;
  enabled: boolean;
  position?: number;
  created_at: string;
  updated_at: string;
}

export interface CreateTransformerParams {
  name: string;
  transformer_type: string;
  phase: string;
  config: Record<string, unknown>;
  enabled: boolean;
}

export type UpdateTransformerParams = CreateTransformerParams;

export interface APIKey {
  id: string;
  organization_id: string;
  name: string;
  key_hash: string;
  key_prefix: string;
  created_at: string;
  revoked_at: string | null;
}

export interface CreateAPIKeyResult {
  api_key: APIKey;
  raw_key: string;
}

export interface AuditLog {
  id: string;
  organization_id: string;
  source_id: string;
  input_hash: string | null;
  // Phase keys ("input"/"output") hold engine results; the reserved keys
  // "transforms" / "output_transforms" hold transformer chain outcomes.
  guard_results: AuditGuardResults | null;
  overall_result: string;
  latency_ms: number;
  created_at: string;
}

export interface AuditGuardResults {
  input?: PhaseGuardResults;
  output?: PhaseGuardResults;
  transforms?: TransformChainResult;
  output_transforms?: TransformChainResult;
}

// TransformChainResult mirrors the data plane's transform.ChainResult.
export interface TransformChainResult {
  steps: TransformStepResult[];
  changed: boolean;
  total_duration_ms: number;
}

export interface TransformStepResult {
  name: string;
  type: string;
  changed: boolean;
  errored?: boolean;
  skipped?: boolean;
  message?: string;
  duration_ms: number;
}

// PhaseGuardResults is one phase's ("input"/"output") engine result within
// an audit log entry.
export interface PhaseGuardResults {
  passed: boolean;
  criteria?: string;
  threshold?: number;
  results: GuardResultEntry[];
  pass_count: number;
  fail_count: number;
  total_duration_ms: number;
}

export interface GuardResultEntry {
  guard_name: string;
  guard_type: string;
  passed: boolean;
  message: string;
  details?: Record<string, unknown>;
  duration_ms: number;
}

export interface ModelInfo {
  id: string;
  provider: string;
}

export interface SourceSummary {
  id: string;
  name: string;
  route: string;
}

export interface Connection {
  source_id: string;
  guardrail_id: string;
}

// Guard config types
export interface BlocklistConfig {
  words: string[];
}

export interface RegexConfig {
  patterns: string[];
  mode: string; // "block" | "require"
}

export interface OpenAIModerationConfig {
  api_key: string;
  categories: string[];
  threshold: number;
  timeout_seconds: number;
}

export interface GuardrailsAIConfig {
  server_url: string;
  guard_name: string;
  timeout_seconds: number;
}

export interface WebhookConfig {
  url: string;
  headers: Record<string, string>;
  timeout_seconds: number;
}

export interface PresidioGuardConfig {
  analyzer_url: string;
  language?: string;
  entities?: string[];
  score_threshold?: number;
  timeout_seconds: number;
}

export interface LLMClassifierConfig {
  base_url: string;
  api_key?: string;
  model: string;
  instructions: string;
  timeout_seconds: number;
}

// Transformer config types
export interface RegexReplaceRule {
  pattern: string;
  replace: string;
}

export interface RegexReplaceConfig {
  rules: RegexReplaceRule[];
}

export interface PresidioConfig {
  analyzer_url: string;
  anonymizer_url?: string;
  mode?: string; // "redact" (default) | "reversible" 
  language?: string;
  entities?: string[];
  score_threshold?: number;
  timeout_seconds: number;
}
