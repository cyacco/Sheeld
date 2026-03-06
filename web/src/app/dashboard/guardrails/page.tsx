"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import type { Guardrail, CreateGuardrailParams } from "@/lib/types";
import * as api from "@/lib/api";
import { GuardrailCard } from "@/components/guardrail-card";
import { GuardrailForm } from "@/components/guardrail-form";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { toast } from "sonner";

export default function GuardrailsPage() {
  const router = useRouter();
  const [guardrails, setGuardrails] = useState<Guardrail[]>([]);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);

  useEffect(() => {
    loadGuardrails();
  }, []);

  async function loadGuardrails() {
    try {
      const data = await api.listGuardrails();
      setGuardrails(data ?? []);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load guardrails");
    } finally {
      setLoading(false);
    }
  }

  async function handleCreate(params: CreateGuardrailParams) {
    try {
      await api.createGuardrail(params);
      toast.success("Guardrail created");
      setOpen(false);
      loadGuardrails();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create guardrail");
    }
  }

  async function handleToggle(guardrail: Guardrail, enabled: boolean) {
    try {
      await api.updateGuardrail(guardrail.id, {
        name: guardrail.name,
        guard_type: guardrail.guard_type,
        phase: guardrail.phase,
        config: guardrail.config,
        enabled,
      });
      loadGuardrails();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to toggle guardrail");
    }
  }

  async function handleDelete(guardrail: Guardrail) {
    try {
      const sources = await api.listGuardrailSources(guardrail.id);
      const safeSource = sources ?? [];
      let message: string;
      if (safeSource.length > 0) {
        const names = safeSource.map((s) => s.name).join(", ");
        message = `This guardrail is attached to ${safeSource.length} source(s): ${names}. Delete anyway?`;
      } else {
        message = `Delete guardrail "${guardrail.name}"?`;
      }
      if (!confirm(message)) return;
      await api.deleteGuardrail(guardrail.id);
      toast.success("Guardrail deleted");
      loadGuardrails();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete guardrail");
    }
  }

  if (loading) {
    return <p className="text-muted-foreground">Loading guardrails...</p>;
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold">Guardrails</h2>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button>Add Guardrail</Button>
          </DialogTrigger>
          <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
            <DialogHeader>
              <DialogTitle>Create Guardrail</DialogTitle>
            </DialogHeader>
            <GuardrailForm onSubmit={handleCreate} submitLabel="Create" />
          </DialogContent>
        </Dialog>
      </div>

      {guardrails.length === 0 ? (
        <p className="text-muted-foreground">
          No guardrails yet. Create one to get started.
        </p>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {guardrails.map((gr) => (
            <GuardrailCard
              key={gr.id}
              guardrail={gr}
              href={`/dashboard/guardrails/${gr.id}`}
              deleteLabel="Delete"
              onToggle={handleToggle}
              onEdit={(gr) => router.push(`/dashboard/guardrails/${gr.id}`)}
              onDelete={handleDelete}
            />
          ))}
        </div>
      )}
    </div>
  );
}
