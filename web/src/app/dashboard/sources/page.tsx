"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { Plus } from "lucide-react";
import { toast } from "sonner";
import type { Source } from "@/lib/types";
import * as api from "@/lib/api";
import { AddSourceWizard } from "@/components/wizards/add-source-wizard";
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

export default function SourcesPage() {
  const router = useRouter();
  const [sources, setSources] = useState<Source[]>([]);
  const [loading, setLoading] = useState(true);
  const [wizardOpen, setWizardOpen] = useState(false);

  const load = useCallback(async () => {
    try {
      setSources((await api.listSources()) ?? []);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load sources");
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
          <h1 className="text-2xl font-bold">Sources</h1>
          <p className="text-sm text-muted-foreground">
            Entry points your app proxies LLM requests through.
          </p>
        </div>
        <Button onClick={() => setWizardOpen(true)}>
          <Plus className="mr-1 h-4 w-4" /> Add source
        </Button>
      </div>

      {loading ? (
        <div className="space-y-2">
          {[...Array(3)].map((_, i) => (
            <Skeleton key={i} className="h-10 w-full" />
          ))}
        </div>
      ) : sources.length === 0 ? (
        <p className="text-muted-foreground">No sources yet.</p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Route</TableHead>
              <TableHead>Model</TableHead>
              <TableHead>Pass criteria</TableHead>
              <TableHead>Status</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {sources.map((s) => (
              <TableRow
                key={s.id}
                className="cursor-pointer"
                onClick={() => router.push(`/dashboard/sources/${s.id}`)}
              >
                <TableCell className="font-medium">{s.name}</TableCell>
                <TableCell className="font-mono text-xs">/{s.route}</TableCell>
                <TableCell>{s.llm_model}</TableCell>
                <TableCell>{s.pass_criteria}</TableCell>
                <TableCell>
                  <Badge variant={s.enabled ? "default" : "outline"}>
                    {s.enabled ? "enabled" : "disabled"}
                  </Badge>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      <AddSourceWizard open={wizardOpen} onOpenChange={setWizardOpen} onCreated={load} />
    </div>
  );
}
