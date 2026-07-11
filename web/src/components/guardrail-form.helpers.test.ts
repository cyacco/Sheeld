import { describe, expect, it } from "vitest";
import type { Guardrail } from "@/lib/types";
import {
  emptyGuardrailDraft,
  guardrailDraftFrom,
  guardrailDraftToParams,
} from "./guardrail-form";

describe("emptyGuardrailDraft", () => {
  it("defaults to a blocklist guard on the input phase", () => {
    const draft = emptyGuardrailDraft();
    expect(draft.guardType).toBe("blocklist");
    expect(draft.phase).toBe("input");
    expect(draft.enabled).toBe(true);
    expect(draft.config).toEqual({ words: [] });
  });

  it("honors an explicit guard type", () => {
    expect(emptyGuardrailDraft("regex").guardType).toBe("regex");
  });
});

describe("guardrailDraft round-trip", () => {
  const guardrail: Guardrail = {
    id: "g1",
    organization_id: "org1",
    name: "Trial Blocklist",
    guard_type: "blocklist",
    phase: "input",
    config: { words: ["forbidden"], mode: "shadow" },
    enabled: true,
    created_at: "2026-07-10T00:00:00Z",
    updated_at: "2026-07-10T00:00:00Z",
  };

  it("maps a guardrail into an editable draft", () => {
    const draft = guardrailDraftFrom(guardrail);
    expect(draft.name).toBe("Trial Blocklist");
    expect(draft.guardType).toBe("blocklist");
    expect(draft.config).toEqual({ words: ["forbidden"], mode: "shadow" });
  });

  it("maps a draft back into create params, preserving config", () => {
    const params = guardrailDraftToParams(guardrailDraftFrom(guardrail));
    expect(params).toEqual({
      name: "Trial Blocklist",
      guard_type: "blocklist",
      phase: "input",
      config: { words: ["forbidden"], mode: "shadow" },
      enabled: true,
    });
  });
});
