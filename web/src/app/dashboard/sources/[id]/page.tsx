"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { ArrowUpRight, Link2Off, Plus } from "lucide-react";
import { toast } from "sonner";
import type { CreateSourceParams, Guardrail, Source } from "@/lib/types";
import * as api from "@/lib/api";
import { guardTypeMeta } from "@/components/guard-type-meta";
import { AuditLogTable } from "@/components/audit-log-table";
import { ResourceHeader } from "@/components/resource-header";
import { SourceForm, sourceDraftFrom, sourceDraftToParams } from "@/components/source-form";
import { AddGuardrailWizard } from "@/components/wizards/add-guardrail-wizard";
import { SourceTransformationsTab } from "@/components/source-transformations-tab";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

export default function SourceDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const [source, setSource] = useState<Source | null>(null);
  const [attached, setAttached] = useState<Guardrail[]>([]);
  const [allGuardrails, setAllGuardrails] = useState<Guardrail[]>([]);
  const [wizardOpen, setWizardOpen] = useState(false);
  const [transformerCount, setTransformerCount] = useState<number | null>(null);

  const load = useCallback(async () => {
    try {
      const [src, attachedList, all] = await Promise.all([
        api.getSource(id),
        api.listGuardrailsBySource(id),
        api.listGuardrails(),
      ]);
      setSource(src);
      setAttached(attachedList ?? []);
      setAllGuardrails(all ?? []);
    } catch {
      toast.error("Failed to load source");
      router.push("/dashboard/sources");
    }
  }, [id, router]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- fetch on mount; state updates happen after awaits
    load();
  }, [load]);

  if (!source) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  const attachedIds = new Set(attached.map((g) => g.id));
  const attachable = allGuardrails.filter((g) => !attachedIds.has(g.id));

  async function handleToggleEnabled(enabled: boolean) {
    try {
      const draft = sourceDraftFrom(source!);
      const updated = await api.updateSource(
        source!.id,
        sourceDraftToParams({ ...draft, enabled }),
      );
      setSource(updated);
      toast.success(enabled ? "Source enabled" : "Source disabled");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to update source");
    }
  }

  async function handleDelete() {
    try {
      await api.deleteSource(source!.id);
      toast.success("Source deleted");
      router.push("/dashboard/sources");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete source");
    }
  }

  async function handleUpdate(params: CreateSourceParams) {
    try {
      const updated = await api.updateSource(source!.id, params);
      setSource(updated);
      toast.success("Source updated");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to update source");
    }
  }

  async function handleAttach(guardrailId: string) {
    try {
      await api.attachGuardrail(guardrailId, source!.id);
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to attach guardrail");
    }
  }

  async function handleDetach(guardrailId: string) {
    try {
      await api.detachGuardrail(guardrailId, source!.id);
      setAttached((prev) => prev.filter((g) => g.id !== guardrailId));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to detach guardrail");
    }
  }

  return (
    <div className="space-y-6">
      <ResourceHeader
        crumbLabel="Sources"
        crumbHref="/dashboard/sources"
        name={source.name}
        badges={[`/${source.route}`, source.llm_model]}
        enabled={source.enabled}
        onToggleEnabled={handleToggleEnabled}
        deleteLabel="This permanently deletes the source and detaches its guardrails. Proxy requests to its route will fail."
        onDelete={handleDelete}
      />

      <Tabs defaultValue="configuration">
        <TabsList>
          <TabsTrigger value="configuration">Configuration</TabsTrigger>
          <TabsTrigger value="guardrails">Guardrails ({attached.length})</TabsTrigger>
          <TabsTrigger value="transformations">
            Transformations{transformerCount !== null ? ` (${transformerCount})` : ""}
          </TabsTrigger>
          <TabsTrigger value="events">Events</TabsTrigger>
        </TabsList>

        <TabsContent value="configuration" className="max-w-2xl pt-4">
          <SourceForm initial={source} onSubmit={handleUpdate} submitLabel="Save changes" />
        </TabsContent>

        <TabsContent value="guardrails" className="space-y-4 pt-4">
          <div className="flex gap-2">
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="outline" size="sm" disabled={attachable.length === 0}>
                  <Plus className="mr-1 h-4 w-4" /> Attach existing
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent>
                {attachable.map((g) => (
                  <DropdownMenuItem key={g.id} onClick={() => handleAttach(g.id)}>
                    {g.name}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
            <Button size="sm" onClick={() => setWizardOpen(true)}>
              <Plus className="mr-1 h-4 w-4" /> Create guardrail
            </Button>
          </div>

          {attached.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No guardrails attached — every proxy request passes straight to the LLM.
            </p>
          ) : (
            <div className="space-y-2">
              {attached.map((g) => {
                const meta = guardTypeMeta(g.guard_type);
                return (
                  <div key={g.id} className="flex items-center gap-3 rounded-lg border p-3">
                    <span className="text-sm font-medium">{g.name}</span>
                    <Badge variant="secondary">{meta?.label ?? g.guard_type}</Badge>
                    <Badge variant="outline">{g.phase}</Badge>
                    {!g.enabled && <Badge variant="outline">disabled</Badge>}
                    <div className="ml-auto flex items-center gap-1">
                      <Button variant="ghost" size="sm" asChild>
                        <Link href={`/dashboard/guardrails/${g.id}`}>
                          <ArrowUpRight className="h-4 w-4" />
                        </Link>
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-destructive"
                        onClick={() => handleDetach(g.id)}
                      >
                        <Link2Off className="mr-1 h-4 w-4" /> Detach
                      </Button>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </TabsContent>

        <TabsContent value="transformations" className="pt-4">
          <SourceTransformationsTab
            sourceId={source.id}
            onCountChange={setTransformerCount}
          />
        </TabsContent>

        <TabsContent value="events" className="pt-4">
          <AuditLogTable sourceId={source.id} />
        </TabsContent>
      </Tabs>

      <AddGuardrailWizard
        open={wizardOpen}
        onOpenChange={setWizardOpen}
        fixedSourceId={source.id}
        onCreated={load}
      />
    </div>
  );
}
