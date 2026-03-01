"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import type {
  Source,
  CreateSourceParams,
  Destination,
  CreateDestinationParams,
} from "@/lib/types";
import * as api from "@/lib/api";
import { SourceForm } from "@/components/source-form";
import { DestinationForm } from "@/components/destination-form";
import { DestinationCard } from "@/components/destination-card";
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
  const [destinations, setDestinations] = useState<Destination[]>([]);
  const [loading, setLoading] = useState(true);
  const [destDialogOpen, setDestDialogOpen] = useState(false);
  const [editingDest, setEditingDest] = useState<Destination | null>(null);

  useEffect(() => {
    loadData();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  async function loadData() {
    try {
      const [src, dests] = await Promise.all([
        api.getSource(id),
        api.listDestinations(id),
      ]);
      setSource(src);
      setDestinations(dests ?? []);
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
    if (!confirm("Delete this source and all its destinations?")) return;
    try {
      await api.deleteSource(id);
      toast.success("Source deleted");
      router.push("/dashboard");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete source");
    }
  }

  async function handleCreateDestination(params: CreateDestinationParams) {
    try {
      await api.createDestination(id, params);
      toast.success("Destination created");
      setDestDialogOpen(false);
      loadData();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create destination");
    }
  }

  async function handleUpdateDestination(params: CreateDestinationParams) {
    if (!editingDest) return;
    try {
      await api.updateDestination(id, editingDest.id, params);
      toast.success("Destination updated");
      setEditingDest(null);
      loadData();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to update destination");
    }
  }

  async function handleToggleDestination(dest: Destination, enabled: boolean) {
    try {
      await api.updateDestination(id, dest.id, {
        name: dest.name,
        guard_type: dest.guard_type,
        phase: dest.phase,
        config: dest.config,
        priority: dest.priority,
        enabled,
      });
      loadData();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to toggle destination");
    }
  }

  async function handleDeleteDestination(dest: Destination) {
    if (!confirm(`Delete destination "${dest.name}"?`)) return;
    try {
      await api.deleteDestination(id, dest.id);
      toast.success("Destination deleted");
      loadData();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete destination");
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
          <TabsTrigger value="destinations">
            Destinations ({destinations.length})
          </TabsTrigger>
        </TabsList>

        <TabsContent value="settings" className="mt-4 max-w-2xl">
          <SourceForm
            initial={source}
            onSubmit={handleUpdateSource}
            submitLabel="Update Source"
          />
        </TabsContent>

        <TabsContent value="destinations" className="mt-4">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-lg font-semibold">Destinations (Guards)</h3>
            <Dialog open={destDialogOpen} onOpenChange={setDestDialogOpen}>
              <DialogTrigger asChild>
                <Button>Add Destination</Button>
              </DialogTrigger>
              <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
                <DialogHeader>
                  <DialogTitle>Add Destination</DialogTitle>
                </DialogHeader>
                <DestinationForm
                  onSubmit={handleCreateDestination}
                  submitLabel="Create"
                />
              </DialogContent>
            </Dialog>
          </div>

          <Separator className="mb-4" />

          {destinations.length === 0 ? (
            <p className="text-muted-foreground">
              No destinations yet. Add one to start guarding this source.
            </p>
          ) : (
            <div className="space-y-3">
              {destinations.map((dest) => (
                <DestinationCard
                  key={dest.id}
                  destination={dest}
                  onToggle={handleToggleDestination}
                  onEdit={setEditingDest}
                  onDelete={handleDeleteDestination}
                />
              ))}
            </div>
          )}

          {/* Edit destination dialog */}
          <Dialog
            open={!!editingDest}
            onOpenChange={(open) => !open && setEditingDest(null)}
          >
            <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
              <DialogHeader>
                <DialogTitle>Edit Destination</DialogTitle>
              </DialogHeader>
              {editingDest && (
                <DestinationForm
                  initial={editingDest}
                  onSubmit={handleUpdateDestination}
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
