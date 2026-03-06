"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import type {
  Source,
  CreateSourceParams,
  Guardrail,
  CreateGuardrailParams,
} from "@/lib/types";
import * as api from "@/lib/api";
import { SourceForm } from "@/components/source-form";
import { GuardrailForm } from "@/components/guardrail-form";
import { GuardrailCard } from "@/components/guardrail-card";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { toast } from "sonner";

export default function SourceDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const [source, setSource] = useState<Source | null>(null);
  const [guardrails, setGuardrails] = useState<Guardrail[]>([]);
  const [loading, setLoading] = useState(true);
  const [guardrailDialogOpen, setGuardrailDialogOpen] = useState(false);
  const [editingGuardrail, setEditingGuardrail] = useState<Guardrail | null>(null);

  useEffect(() => {
    loadData();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  async function loadData() {
    try {
      const [src, grs] = await Promise.all([
        api.getSource(id),
        api.listGuardrailsBySource(id),
      ]);
      setSource(src);
      setGuardrails(grs ?? []);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load source");
    } finally {
      setLoading(false);
    }
  }

  async function handleUpdateSource(params: CreateSourceParams) {
    try {
      const updated = await api.updateSource(id, params);
      setSource(updated);
      toast.success("Source updated");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to update source");
    }
  }

  async function handleDeleteSource() {
    if (!confirm("Delete this source?")) return;
    try {
      await api.deleteSource(id);
      toast.success("Source deleted");
      router.push("/dashboard");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete source");
    }
  }

  async function handleCreateGuardrail(params: CreateGuardrailParams) {
    try {
      const guardrail = await api.createGuardrail(params);
      await api.attachGuardrail(guardrail.id, id);
      toast.success("Guardrail created");
      setGuardrailDialogOpen(false);
      loadData();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create guardrail");
    }
  }

  async function handleUpdateGuardrail(params: CreateGuardrailParams) {
    if (!editingGuardrail) return;
    try {
      await api.updateGuardrail(editingGuardrail.id, params);
      toast.success("Guardrail updated");
      setEditingGuardrail(null);
      loadData();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to update guardrail");
    }
  }

  async function handleToggleGuardrail(guardrail: Guardrail, enabled: boolean) {
    try {
      await api.updateGuardrail(guardrail.id, {
        name: guardrail.name,
        guard_type: guardrail.guard_type,
        phase: guardrail.phase,
        config: guardrail.config,
        enabled,
      });
      loadData();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to toggle guardrail");
    }
  }

  async function handleDeleteGuardrail(guardrail: Guardrail) {
    if (!confirm(`Remove guardrail "${guardrail.name}" from this source?`)) return;
    try {
      await api.detachGuardrail(guardrail.id, id);
      toast.success("Guardrail removed");
      loadData();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to remove guardrail");
    }
  }

  if (loading) return <p className="text-muted-foreground">Loading...</p>;
  if (!source) return <p className="text-muted-foreground">Source not found.</p>;

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h2 className="text-2xl font-bold">{source.name}</h2>
          <p className="text-sm text-muted-foreground font-mono">/{source.route}</p>
        </div>
        <Button variant="destructive" size="sm" onClick={handleDeleteSource}>
          Delete Source
        </Button>
      </div>

      <Tabs defaultValue="settings">
        <TabsList>
          <TabsTrigger value="settings">Settings</TabsTrigger>
          <TabsTrigger value="guardrails">
            Guardrails ({guardrails.length})
          </TabsTrigger>
        </TabsList>

        <TabsContent value="settings" className="mt-4 max-w-2xl">
          <SourceForm
            initial={source}
            onSubmit={handleUpdateSource}
            submitLabel="Update Source"
          />
        </TabsContent>

        <TabsContent value="guardrails" className="mt-4">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-lg font-semibold">Guardrails</h3>
            <Dialog open={guardrailDialogOpen} onOpenChange={setGuardrailDialogOpen}>
              <DialogTrigger asChild>
                <Button>Add Guardrail</Button>
              </DialogTrigger>
              <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
                <DialogHeader>
                  <DialogTitle>Add Guardrail</DialogTitle>
                </DialogHeader>
                <GuardrailForm
                  onSubmit={handleCreateGuardrail}
                  submitLabel="Create"
                />
              </DialogContent>
            </Dialog>
          </div>

          <Separator className="mb-4" />

          {guardrails.length === 0 ? (
            <p className="text-muted-foreground">
              No guardrails yet. Add one to start guarding this source.
            </p>
          ) : (
            <div className="space-y-3">
              {guardrails.map((gr) => (
                <GuardrailCard
                  key={gr.id}
                  guardrail={gr}
                  onToggle={handleToggleGuardrail}
                  onEdit={setEditingGuardrail}
                  onDelete={handleDeleteGuardrail}
                />
              ))}
            </div>
          )}

          {/* Edit guardrail dialog */}
          <Dialog
            open={!!editingGuardrail}
            onOpenChange={(open) => !open && setEditingGuardrail(null)}
          >
            <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
              <DialogHeader>
                <DialogTitle>Edit Guardrail</DialogTitle>
              </DialogHeader>
              {editingGuardrail && (
                <GuardrailForm
                  initial={editingGuardrail}
                  onSubmit={handleUpdateGuardrail}
                  submitLabel="Update"
                />
              )}
            </DialogContent>
          </Dialog>
        </TabsContent>
      </Tabs>
    </div>
  );
}
