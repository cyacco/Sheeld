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
  pass_criteria: string;
  pass_threshold?: number;
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
  pass_criteria: string;
  pass_threshold?: number;
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
  guard_results: Record<string, PhaseGuardResults> | null;
  overall_result: string;
  latency_ms: number;
  created_at: string;
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
  fail_open: boolean;
}
