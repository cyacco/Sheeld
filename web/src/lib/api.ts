import type {
  RegisterResult,
  LoginResult,
  Source,
  CreateSourceParams,
  UpdateSourceParams,
  Guardrail,
  CreateGuardrailParams,
  UpdateGuardrailParams,
  APIKey,
  CreateAPIKeyResult,
  AuditLog,
  ModelInfo,
} from "./types";

const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

class APIError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
    this.name = "APIError";
  }
}

async function request<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const token =
    typeof window !== "undefined" ? localStorage.getItem("token") : null;

  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...((options.headers as Record<string, string>) || {}),
  };

  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  const res = await fetch(`${API_URL}${path}`, {
    ...options,
    headers,
  });

  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new APIError(res.status, body.error || body.message || res.statusText);
  }

  if (res.status === 204) {
    return undefined as T;
  }

  return res.json();
}

// Auth
export async function register(
  orgName: string,
  email: string,
  password: string,
): Promise<RegisterResult> {
  return request<RegisterResult>("/v1/auth/register", {
    method: "POST",
    body: JSON.stringify({ org_name: orgName, email, password }),
  });
}

export async function login(
  email: string,
  password: string,
): Promise<LoginResult> {
  return request<LoginResult>("/v1/auth/login", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
}

// Sources
export async function listSources(): Promise<Source[]> {
  return request<Source[]>("/v1/sources");
}

export async function getSource(id: string): Promise<Source> {
  return request<Source>(`/v1/sources/${id}`);
}

export async function createSource(
  params: CreateSourceParams,
): Promise<Source> {
  return request<Source>("/v1/sources", {
    method: "POST",
    body: JSON.stringify(params),
  });
}

export async function updateSource(
  id: string,
  params: UpdateSourceParams,
): Promise<Source> {
  return request<Source>(`/v1/sources/${id}`, {
    method: "PUT",
    body: JSON.stringify(params),
  });
}

export async function deleteSource(id: string): Promise<void> {
  return request<void>(`/v1/sources/${id}`, { method: "DELETE" });
}

// Guardrails
export async function listGuardrails(): Promise<Guardrail[]> {
  return request<Guardrail[]>("/v1/guardrails");
}

export async function listGuardrailsBySource(
  sourceId: string,
): Promise<Guardrail[]> {
  return request<Guardrail[]>(`/v1/sources/${sourceId}/guardrails`);
}

export async function getGuardrail(id: string): Promise<Guardrail> {
  return request<Guardrail>(`/v1/guardrails/${id}`);
}

export async function createGuardrail(
  params: CreateGuardrailParams,
): Promise<Guardrail> {
  return request<Guardrail>("/v1/guardrails", {
    method: "POST",
    body: JSON.stringify(params),
  });
}

export async function updateGuardrail(
  id: string,
  params: UpdateGuardrailParams,
): Promise<Guardrail> {
  return request<Guardrail>(`/v1/guardrails/${id}`, {
    method: "PUT",
    body: JSON.stringify(params),
  });
}

export async function deleteGuardrail(id: string): Promise<void> {
  return request<void>(`/v1/guardrails/${id}`, { method: "DELETE" });
}

export async function attachGuardrail(
  guardrailId: string,
  sourceId: string,
): Promise<void> {
  return request<void>(`/v1/guardrails/${guardrailId}/sources`, {
    method: "POST",
    body: JSON.stringify({ source_id: sourceId }),
  });
}

export async function detachGuardrail(
  guardrailId: string,
  sourceId: string,
): Promise<void> {
  return request<void>(`/v1/guardrails/${guardrailId}/sources/${sourceId}`, {
    method: "DELETE",
  });
}

// API Keys
export async function listAPIKeys(): Promise<APIKey[]> {
  return request<APIKey[]>("/v1/auth/api-keys");
}

export async function createAPIKey(
  name: string,
): Promise<CreateAPIKeyResult> {
  return request<CreateAPIKeyResult>("/v1/auth/api-keys", {
    method: "POST",
    body: JSON.stringify({ name }),
  });
}

export async function revokeAPIKey(id: string): Promise<void> {
  return request<void>(`/v1/auth/api-keys/${id}`, { method: "DELETE" });
}

// Models
export async function listModels(provider?: string): Promise<ModelInfo[]> {
  const qs = provider ? `?provider=${encodeURIComponent(provider)}` : "";
  return request<ModelInfo[]>(`/v1/models${qs}`);
}

// Audit Logs
export async function listAuditLogs(params: {
  limit?: number;
  offset?: number;
  source_id?: string;
}): Promise<AuditLog[]> {
  const searchParams = new URLSearchParams();
  if (params.limit) searchParams.set("limit", String(params.limit));
  if (params.offset) searchParams.set("offset", String(params.offset));
  if (params.source_id) searchParams.set("source_id", params.source_id);
  const qs = searchParams.toString();
  return request<AuditLog[]>(`/v1/audit-logs${qs ? `?${qs}` : ""}`);
}

export { APIError };
