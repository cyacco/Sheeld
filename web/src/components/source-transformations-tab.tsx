"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { ArrowDown, ArrowUp, ArrowUpRight, Link2Off, Plus } from "lucide-react";
import { toast } from "sonner";
import type { Transformer } from "@/lib/types";
import * as api from "@/lib/api";
import { transformerTypeMeta } from "@/components/transformer-type-meta";
import { AddTransformerWizard } from "@/components/wizards/add-transformer-wizard";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

interface SourceTransformationsTabProps {
  sourceId: string;
  /** Called whenever the attached count changes so the parent tab label stays fresh. */
  onCountChange?: (count: number) => void;
}

// Ordered transformation chain for a source: attach, create, reorder
// (up/down), detach. Reorder replaces the whole chain via PUT.
export function SourceTransformationsTab({
  sourceId,
  onCountChange,
}: SourceTransformationsTabProps) {
  const [chain, setChain] = useState<Transformer[]>([]);
  const [all, setAll] = useState<Transformer[]>([]);
  const [wizardOpen, setWizardOpen] = useState(false);

  const load = useCallback(async () => {
    try {
      const [attached, allList] = await Promise.all([
        api.listTransformersBySource(sourceId),
        api.listTransformers(),
      ]);
      setChain(attached ?? []);
      setAll(allList ?? []);
      onCountChange?.((attached ?? []).length);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load transformations");
    }
  }, [sourceId, onCountChange]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- fetch on mount; state updates happen after awaits
    load();
  }, [load]);

  const chainIds = new Set(chain.map((t) => t.id));
  const attachable = all.filter((t) => !chainIds.has(t.id));

  async function handleAttach(transformerId: string) {
    try {
      await api.attachTransformer(transformerId, sourceId);
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to attach transformation");
    }
  }

  async function handleDetach(transformerId: string) {
    try {
      await api.detachTransformer(transformerId, sourceId);
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to detach transformation");
    }
  }

  async function handleMove(index: number, delta: -1 | 1) {
    const next = [...chain];
    const [item] = next.splice(index, 1);
    next.splice(index + delta, 0, item);
    const prev = chain;
    setChain(next);
    try {
      await api.setSourceTransformers(sourceId, next.map((t) => t.id));
    } catch (err) {
      setChain(prev);
      toast.error(err instanceof Error ? err.message : "Failed to reorder");
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex gap-2">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" size="sm" disabled={attachable.length === 0}>
              <Plus className="mr-1 h-4 w-4" /> Attach existing
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent>
            {attachable.map((t) => (
              <DropdownMenuItem key={t.id} onClick={() => handleAttach(t.id)}>
                {t.name}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
        <Button size="sm" onClick={() => setWizardOpen(true)}>
          <Plus className="mr-1 h-4 w-4" /> Create transformation
        </Button>
      </div>

      {chain.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          No transformations attached — requests reach the guardrails unchanged.
        </p>
      ) : (
        <div className="space-y-2">
          <p className="text-xs text-muted-foreground">
            Runs top to bottom before input guardrails and the LLM.
          </p>
          {chain.map((t, i) => {
            const meta = transformerTypeMeta(t.transformer_type);
            return (
              <div key={t.id} className="flex items-center gap-3 rounded-lg border p-3">
                <span className="w-5 text-center font-mono text-xs text-muted-foreground">
                  {i + 1}
                </span>
                <span className="text-sm font-medium">{t.name}</span>
                <Badge variant="secondary">{meta?.label ?? t.transformer_type}</Badge>
                {!t.enabled && <Badge variant="outline">disabled</Badge>}
                <div className="ml-auto flex items-center gap-1">
                  <Button
                    variant="ghost"
                    size="icon"
                    disabled={i === 0}
                    onClick={() => handleMove(i, -1)}
                    aria-label="Move up"
                  >
                    <ArrowUp className="h-4 w-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    disabled={i === chain.length - 1}
                    onClick={() => handleMove(i, 1)}
                    aria-label="Move down"
                  >
                    <ArrowDown className="h-4 w-4" />
                  </Button>
                  <Button variant="ghost" size="sm" asChild>
                    <Link href={`/dashboard/transformations/${t.id}`}>
                      <ArrowUpRight className="h-4 w-4" />
                    </Link>
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="text-destructive"
                    onClick={() => handleDetach(t.id)}
                  >
                    <Link2Off className="mr-1 h-4 w-4" /> Detach
                  </Button>
                </div>
              </div>
            );
          })}
        </div>
      )}

      <AddTransformerWizard
        open={wizardOpen}
        onOpenChange={setWizardOpen}
        fixedSourceId={sourceId}
        onCreated={load}
      />
    </div>
  );
}
