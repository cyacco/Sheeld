"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { Plus } from "lucide-react";
import { toast } from "sonner";
import type { Transformer } from "@/lib/types";
import * as api from "@/lib/api";
import { transformerTypeMeta } from "@/components/transformer-type-meta";
import { AddTransformerWizard } from "@/components/wizards/add-transformer-wizard";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

export default function TransformationsPage() {
  const router = useRouter();
  const [transformers, setTransformers] = useState<Transformer[]>([]);
  const [loading, setLoading] = useState(true);
  const [wizardOpen, setWizardOpen] = useState(false);

  const load = useCallback(async () => {
    try {
      setTransformers((await api.listTransformers()) ?? []);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load transformations");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Transformations</h1>
          <p className="text-sm text-muted-foreground">
            Input rewriters that run in order before guardrails and the LLM.
          </p>
        </div>
        <Button onClick={() => setWizardOpen(true)}>
          <Plus className="mr-1 h-4 w-4" /> Add transformation
        </Button>
      </div>

      {loading ? (
        <div className="space-y-2">
          {[...Array(3)].map((_, i) => (
            <Skeleton key={i} className="h-10 w-full" />
          ))}
        </div>
      ) : transformers.length === 0 ? (
        <p className="text-muted-foreground">No transformations yet.</p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Type</TableHead>
              <TableHead>Phase</TableHead>
              <TableHead>Status</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {transformers.map((t) => (
              <TableRow
                key={t.id}
                className="cursor-pointer"
                onClick={() => router.push(`/dashboard/transformations/${t.id}`)}
              >
                <TableCell className="font-medium">{t.name}</TableCell>
                <TableCell>
                  <Badge variant="secondary">
                    {transformerTypeMeta(t.transformer_type)?.label ??
                      t.transformer_type}
                  </Badge>
                </TableCell>
                <TableCell>{t.phase}</TableCell>
                <TableCell>
                  <Badge variant={t.enabled ? "default" : "outline"}>
                    {t.enabled ? "enabled" : "disabled"}
                  </Badge>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      <AddTransformerWizard
        open={wizardOpen}
        onOpenChange={setWizardOpen}
        onCreated={load}
      />
    </div>
  );
}
