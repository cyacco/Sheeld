"use client";

import { useState } from "react";
import { CheckCircle2, XCircle } from "lucide-react";
import type { GuardResult } from "@/lib/types";
import * as api from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";

// GuardTestPanel runs a guard against sample text (dry-run) and shows whether
// the input would pass or be rejected — without touching live proxy traffic.
export function GuardTestPanel({ guardrailId }: { guardrailId: string }) {
  const [input, setInput] = useState("");
  const [result, setResult] = useState<GuardResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function runTest() {
    setLoading(true);
    setError(null);
    setResult(null);
    try {
      setResult(await api.testGuardrail(guardrailId, input));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Test failed");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="max-w-2xl space-y-4">
      <div className="space-y-2">
        <Label htmlFor="test-input">Sample input</Label>
        <Textarea
          id="test-input"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="Type text to run against this guard…"
          rows={4}
        />
        <p className="text-xs text-muted-foreground">
          Runs the guard exactly as the proxy would, but only here — no traffic
          is affected and nothing is logged to the audit trail.
        </p>
      </div>

      <Button onClick={runTest} disabled={loading || input.length === 0}>
        {loading ? "Running…" : "Run test"}
      </Button>

      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/5 p-3 text-sm text-destructive">
          {error}
        </div>
      )}

      {result && (
        <div className="space-y-2 rounded-lg border p-4">
          <div className="flex items-center gap-2">
            {result.passed ? (
              <>
                <CheckCircle2 className="h-5 w-5 text-green-600" />
                <span className="font-medium text-green-600">Passed</span>
              </>
            ) : (
              <>
                <XCircle className="h-5 w-5 text-destructive" />
                <span className="font-medium text-destructive">Rejected</span>
              </>
            )}
          </div>
          <p className="text-sm text-muted-foreground">{result.message}</p>
          {result.details && Object.keys(result.details).length > 0 && (
            <pre className="overflow-x-auto rounded bg-muted p-2 text-xs">
              {JSON.stringify(result.details, null, 2)}
            </pre>
          )}
        </div>
      )}
    </div>
  );
}
