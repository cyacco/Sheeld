import { describe, expect, it } from "vitest";
import type { BlocklistConfig, LLMClassifierConfig } from "@/lib/types";
import { GUARD_TYPES, defaultConfig, guardTypeMeta } from "./guard-type-meta";

describe("guardTypeMeta", () => {
  it("resolves a known guard type", () => {
    expect(guardTypeMeta("blocklist")?.value).toBe("blocklist");
  });

  it("returns undefined for an unknown type", () => {
    expect(guardTypeMeta("nope")).toBeUndefined();
  });

  it("has metadata for every catalog entry", () => {
    for (const gt of GUARD_TYPES) {
      expect(guardTypeMeta(gt.value)).toBe(gt);
    }
  });
});

describe("defaultConfig", () => {
  it("returns an empty blocklist word list", () => {
    expect(defaultConfig("blocklist")).toEqual({ words: [] } satisfies BlocklistConfig);
  });

  it("seeds the llm_classifier with sensible defaults", () => {
    const cfg = defaultConfig("llm_classifier") as unknown as LLMClassifierConfig;
    expect(cfg.base_url).toBe("https://api.openai.com/v1");
    expect(cfg.model).toBe("gpt-4o-mini");
    expect(cfg.timeout_seconds).toBe(15);
  });

  it("returns an empty object for an unknown guard type", () => {
    expect(defaultConfig("nope")).toEqual({});
  });
});
