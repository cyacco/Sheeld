"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { ArrowUpRight, Link2Off } from "lucide-react";
import { toast } from "sonner";
import type { CreateTransformerParams, SourceSummary, Transformer } from "@/lib/types";
import * as api from "@/lib/api";
import { transformerTypeMeta } from "@/components/transformer-type-meta";
import {
  TransformerForm,
  transformerDraftFrom,
  transformerDraftToParams,
} from "@/components/transformer-form";
import { ResourceHeader } from "@/components/resource-header";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

export default function TransformationDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const [transformer, setTransformer] = useState<Transformer | null>(null);
  const [sources, setSources] = useState<SourceSummary[]>([]);

  const load = useCallback(async () => {
    try {
      const [t, srcs] = await Promise.all([
        api.getTransformer(id),
        api.listTransformerSources(id),
      ]);
      setTransformer(t);
      setSources(srcs ?? []);
    } catch {
      toast.error("Failed to load transformation");
      router.push("/dashboard/transformations");
    }
  }, [id, router]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- fetch on mount; state updates happen after awaits
    load();
  }, [load]);

  if (!transformer) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  const meta = transformerTypeMeta(transformer.transformer_type);

  async function handleToggleEnabled(enabled: boolean) {
    try {
      const draft = transformerDraftFrom(transformer!);
      const updated = await api.updateTransformer(
        transformer!.id,
        transformerDraftToParams({ ...draft, enabled }),
      );
      setTransformer(updated);
      toast.success(enabled ? "Transformation enabled" : "Transformation disabled");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to update transformation");
    }
  }

  async function handleDelete() {
    try {
      await api.deleteTransformer(transformer!.id);
      toast.success("Transformation deleted");
      router.push("/dashboard/transformations");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete transformation");
    }
  }

  async function handleUpdate(params: CreateTransformerParams) {
    try {
      const updated = await api.updateTransformer(transformer!.id, params);
      setTransformer(updated);
      toast.success("Transformation updated");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to update transformation");
    }
  }

  async function handleDetach(sourceId: string) {
    try {
      await api.detachTransformer(transformer!.id, sourceId);
      setSources((prev) => prev.filter((s) => s.id !== sourceId));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to detach");
    }
  }

  return (
    <div className="space-y-6">
      <ResourceHeader
        crumbLabel="Transformations"
        crumbHref="/dashboard/transformations"
        name={transformer.name}
        badges={[meta?.label ?? transformer.transformer_type, transformer.phase]}
        enabled={transformer.enabled}
        onToggleEnabled={handleToggleEnabled}
        deleteLabel="This permanently deletes the transformation and removes it from all source chains."
        onDelete={handleDelete}
      />

      <Tabs defaultValue="configuration">
        <TabsList>
          <TabsTrigger value="configuration">Configuration</TabsTrigger>
          <TabsTrigger value="sources">Connected sources ({sources.length})</TabsTrigger>
        </TabsList>

        <TabsContent value="configuration" className="max-w-2xl pt-4">
          <TransformerForm
            initial={transformer}
            onSubmit={handleUpdate}
            submitLabel="Save changes"
          />
        </TabsContent>

        <TabsContent value="sources" className="space-y-2 pt-4">
          {sources.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              Not attached to any sources — wire it up from a source&apos;s
              Transformations tab.
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
