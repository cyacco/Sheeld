"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { ArrowUpRight, Link2Off } from "lucide-react";
import { toast } from "sonner";
import type { CreateGuardrailParams, Guardrail, SourceSummary } from "@/lib/types";
import * as api from "@/lib/api";
import { guardTypeMeta } from "@/components/guard-type-meta";
import { GuardrailForm, guardrailDraftFrom, guardrailDraftToParams } from "@/components/guardrail-form";
import { GuardTestPanel } from "@/components/guard-test-panel";
import { ResourceHeader } from "@/components/resource-header";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

export default function GuardrailDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const [guardrail, setGuardrail] = useState<Guardrail | null>(null);
  const [sources, setSources] = useState<SourceSummary[]>([]);

  const load = useCallback(async () => {
    try {
      const [g, srcs] = await Promise.all([
        api.getGuardrail(id),
        api.listGuardrailSources(id),
      ]);
      setGuardrail(g);
      setSources(srcs ?? []);
    } catch {
      toast.error("Failed to load guardrail");
      router.push("/dashboard/guardrails");
    }
  }, [id, router]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- fetch on mount; state updates happen after awaits
    load();
  }, [load]);

  if (!guardrail) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  const meta = guardTypeMeta(guardrail.guard_type);

  async function handleToggleEnabled(enabled: boolean) {
    try {
      const draft = guardrailDraftFrom(guardrail!);
      const updated = await api.updateGuardrail(
        guardrail!.id,
        guardrailDraftToParams({ ...draft, enabled }),
      );
      setGuardrail(updated);
      toast.success(enabled ? "Guardrail enabled" : "Guardrail disabled");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to update guardrail");
    }
  }

  async function handleDelete() {
    try {
      await api.deleteGuardrail(guardrail!.id);
      toast.success("Guardrail deleted");
      router.push("/dashboard/guardrails");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete guardrail");
    }
  }

  async function handleUpdate(params: CreateGuardrailParams) {
    try {
      const updated = await api.updateGuardrail(guardrail!.id, params);
      setGuardrail(updated);
      toast.success("Guardrail updated");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to update guardrail");
    }
  }

  async function handleDetach(sourceId: string) {
    try {
      await api.detachGuardrail(guardrail!.id, sourceId);
      setSources((prev) => prev.filter((s) => s.id !== sourceId));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to detach");
    }
  }

  return (
    <div className="space-y-6">
      <ResourceHeader
        crumbLabel="Guardrails"
        crumbHref="/dashboard/guardrails"
        name={guardrail.name}
        badges={[meta?.label ?? guardrail.guard_type, guardrail.phase]}
        enabled={guardrail.enabled}
        onToggleEnabled={handleToggleEnabled}
        deleteLabel="This permanently deletes the guardrail and detaches it from all sources."
        onDelete={handleDelete}
      />

      <Tabs defaultValue="configuration">
        <TabsList>
          <TabsTrigger value="configuration">Configuration</TabsTrigger>
          <TabsTrigger value="test">Test</TabsTrigger>
          <TabsTrigger value="sources">Connected sources ({sources.length})</TabsTrigger>
        </TabsList>

        <TabsContent value="configuration" className="max-w-2xl pt-4">
          <GuardrailForm
            initial={guardrail}
            onSubmit={handleUpdate}
            submitLabel="Save changes"
          />
        </TabsContent>

        <TabsContent value="test" className="pt-4">
          <GuardTestPanel guardrailId={guardrail.id} />
        </TabsContent>

        <TabsContent value="sources" className="space-y-2 pt-4">
          {sources.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              Not attached to any sources — wire it up from the Connections page.
            </p>
          ) : (
            sources.map((s) => (
              <div key={s.id} className="flex items-center gap-3 rounded-lg border p-3">
                <span className="text-sm font-medium">{s.name}</span>
                <span className="font-mono text-xs text-muted-foreground">/{s.route}</span>
                <div className="ml-auto flex items-center gap-1">
                  <Button variant="ghost" size="sm" asChild>
                    <Link href={`/dashboard/sources/${s.id}`}>
                      <ArrowUpRight className="h-4 w-4" />
                    </Link>
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="text-destructive"
                    onClick={() => handleDetach(s.id)}
                  >
                    <Link2Off className="mr-1 h-4 w-4" /> Detach
                  </Button>
                </div>
              </div>
            ))
          )}
        </TabsContent>
      </Tabs>
    </div>
  );
}
