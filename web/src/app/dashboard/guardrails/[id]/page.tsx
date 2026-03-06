"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import type {
  Guardrail,
  CreateGuardrailParams,
  SourceSummary,
} from "@/lib/types";
import * as api from "@/lib/api";
import { GuardrailForm } from "@/components/guardrail-form";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { toast } from "sonner";

export default function GuardrailDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const [guardrail, setGuardrail] = useState<Guardrail | null>(null);
  const [sources, setSources] = useState<SourceSummary[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadData();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  async function loadData() {
    try {
      const [gr, srcs] = await Promise.all([
        api.getGuardrail(id),
        api.listGuardrailSources(id),
      ]);
      setGuardrail(gr);
      setSources(srcs ?? []);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load guardrail");
    } finally {
      setLoading(false);
    }
  }

  async function handleUpdate(params: CreateGuardrailParams) {
    try {
      const updated = await api.updateGuardrail(id, params);
      setGuardrail(updated);
      toast.success("Guardrail updated");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to update guardrail");
    }
  }

  async function handleDelete() {
    let message: string;
    if (sources.length > 0) {
      const names = sources.map((s) => s.name).join(", ");
      message = `This guardrail is attached to ${sources.length} source(s): ${names}. Delete anyway?`;
    } else {
      message = `Delete guardrail "${guardrail?.name}"?`;
    }
    if (!confirm(message)) return;
    try {
      await api.deleteGuardrail(id);
      toast.success("Guardrail deleted");
      router.push("/dashboard/guardrails");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete guardrail");
    }
  }

  if (loading) return <p className="text-muted-foreground">Loading...</p>;
  if (!guardrail) return <p className="text-muted-foreground">Guardrail not found.</p>;

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-3">
          <h2 className="text-2xl font-bold">{guardrail.name}</h2>
          <Badge variant="outline">{guardrail.guard_type}</Badge>
          <Badge variant="secondary">{guardrail.phase}</Badge>
        </div>
        <Button variant="destructive" size="sm" onClick={handleDelete}>
          Delete Guardrail
        </Button>
      </div>

      <div className="max-w-2xl">
        <GuardrailForm
          initial={guardrail}
          onSubmit={handleUpdate}
          submitLabel="Update Guardrail"
        />
      </div>

      <Separator className="my-6" />

      <h3 className="text-lg font-semibold mb-3">Attached Sources</h3>
      {sources.length === 0 ? (
        <p className="text-muted-foreground">
          This guardrail is not attached to any sources.
        </p>
      ) : (
        <div className="space-y-2">
          {sources.map((source) => (
            <div key={source.id} className="flex items-center gap-3 rounded-md border p-3">
              <Link
                href={`/dashboard/sources/${source.id}`}
                className="font-medium hover:underline"
              >
                {source.name}
              </Link>
              <span className="text-sm text-muted-foreground font-mono">
                /{source.route}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
