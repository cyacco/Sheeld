"use client";

import { useEffect, useState } from "react";
import type { Source, CreateSourceParams } from "@/lib/types";
import * as api from "@/lib/api";
import { SourceCard } from "@/components/source-card";
import { SourceForm } from "@/components/source-form";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { toast } from "sonner";

export default function DashboardPage() {
  const [sources, setSources] = useState<Source[]>([]);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);

  useEffect(() => {
    loadSources();
  }, []);

  async function loadSources() {
    try {
      const data = await api.listSources();
      setSources(data ?? []);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load sources");
    } finally {
      setLoading(false);
    }
  }

  async function handleCreate(params: CreateSourceParams) {
    try {
      await api.createSource(params);
      toast.success("Source created");
      setOpen(false);
      loadSources();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create source");
    }
  }

  if (loading) {
    return <p className="text-muted-foreground">Loading sources...</p>;
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold">Sources</h2>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button>Create Source</Button>
          </DialogTrigger>
          <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
            <DialogHeader>
              <DialogTitle>Create Source</DialogTitle>
            </DialogHeader>
            <SourceForm onSubmit={handleCreate} submitLabel="Create" />
          </DialogContent>
        </Dialog>
      </div>

      {sources.length === 0 ? (
        <p className="text-muted-foreground">
          No sources yet. Create one to get started.
        </p>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {sources.map((source) => (
            <SourceCard key={source.id} source={source} />
          ))}
        </div>
      )}
    </div>
  );
}
