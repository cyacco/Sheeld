"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { PlugZap, Plus, Shield } from "lucide-react";
import { toast } from "sonner";
import type { Connection, Guardrail, Source } from "@/lib/types";
import {
  attachGuardrail,
  detachGuardrail,
  listConnections,
  listGuardrails,
  listSources,
} from "@/lib/api";
import { guardTypeMeta } from "@/components/guard-type-meta";
import { NodeCard } from "@/components/connections/node-card";
import { Wires, type Wire } from "@/components/connections/wires";
import { AddSourceWizard } from "@/components/wizards/add-source-wizard";
import { AddGuardrailWizard } from "@/components/wizards/add-guardrail-wizard";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";

type Selection = { kind: "source" | "guardrail"; id: string } | null;

function connKey(sourceId: string, guardrailId: string) {
  return `${sourceId}:${guardrailId}`;
}

export function ConnectionsBoard() {
  const [sources, setSources] = useState<Source[]>([]);
  const [guardrails, setGuardrails] = useState<Guardrail[]>([]);
  const [connections, setConnections] = useState<Connection[]>([]);
  const [loading, setLoading] = useState(true);
  const [selection, setSelection] = useState<Selection>(null);
  const [version, setVersion] = useState(0);
  const [sourceWizardOpen, setSourceWizardOpen] = useState(false);
  const [guardrailWizardOpen, setGuardrailWizardOpen] = useState(false);

  const containerRef = useRef<HTMLDivElement | null>(null);
  const nodeRefs = useRef(new Map<string, HTMLElement>()).current;

  const load = useCallback(async () => {
    try {
      const [srcs, guards, conns] = await Promise.all([
        listSources(),
        listGuardrails(),
        listConnections(),
      ]);
      setSources(srcs);
      setGuardrails(guards);
      setConnections(conns);
      setVersion((v) => v + 1);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load connections");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  // Esc clears selection.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") setSelection(null);
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const connected = useMemo(() => {
    const set = new Set<string>();
    for (const c of connections) set.add(connKey(c.source_id, c.guardrail_id));
    return set;
  }, [connections]);

  const enabledById = useMemo(() => {
    const m = new Map<string, boolean>();
    for (const s of sources) m.set(s.id, s.enabled);
    for (const g of guardrails) m.set(g.id, g.enabled);
    return m;
  }, [sources, guardrails]);

  const wires: Wire[] = useMemo(
    () =>
      connections.map((c) => ({
        key: connKey(c.source_id, c.guardrail_id),
        fromId: c.source_id,
        toId: c.guardrail_id,
        highlighted:
          selection !== null &&
          (selection.id === c.source_id || selection.id === c.guardrail_id),
        dashed:
          enabledById.get(c.source_id) === false ||
          enabledById.get(c.guardrail_id) === false,
      })),
    [connections, selection, enabledById],
  );

  async function toggleAttach(sourceId: string, guardrailId: string) {
    const key = connKey(sourceId, guardrailId);
    const isAttached = connected.has(key);
    // Optimistic update; rollback on error.
    const prev = connections;
    setConnections(
      isAttached
        ? prev.filter((c) => connKey(c.source_id, c.guardrail_id) !== key)
        : [...prev, { source_id: sourceId, guardrail_id: guardrailId }],
    );
    setVersion((v) => v + 1);
    try {
      if (isAttached) await detachGuardrail(guardrailId, sourceId);
      else await attachGuardrail(guardrailId, sourceId);
    } catch (err) {
      setConnections(prev);
      setVersion((v) => v + 1);
      toast.error(err instanceof Error ? err.message : "Failed to update connection");
    }
  }

  const registerRef = useCallback(
    (id: string) => (el: HTMLElement | null) => {
      if (el) nodeRefs.set(id, el);
      else nodeRefs.delete(id);
    },
    [nodeRefs],
  );

  if (loading) {
    return (
      <div className="grid grid-cols-2 gap-24">
        <div className="space-y-3">
          {[...Array(3)].map((_, i) => (
            <Skeleton key={i} className="h-16 w-full" />
          ))}
        </div>
        <div className="space-y-3">
          {[...Array(3)].map((_, i) => (
            <Skeleton key={i} className="h-16 w-full" />
          ))}
        </div>
      </div>
    );
  }

  return (
    <>
      <div
        ref={containerRef}
        className="relative"
        onClick={(e) => {
          // Background click clears selection.
          if (e.target === e.currentTarget) setSelection(null);
        }}
      >
        <Wires
          containerRef={containerRef}
          nodeRefs={nodeRefs}
          wires={wires}
          version={version}
        />
        <div className="grid grid-cols-2 gap-24">
          {/* Sources column */}
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <h2 className="text-sm font-semibold text-muted-foreground">SOURCES</h2>
              <Button size="sm" variant="outline" onClick={() => setSourceWizardOpen(true)}>
                <Plus className="mr-1 h-4 w-4" /> Add source
              </Button>
            </div>
            {sources.length === 0 && (
              <button
                onClick={() => setSourceWizardOpen(true)}
                className="flex w-full flex-col items-center gap-2 rounded-lg border border-dashed p-8 text-muted-foreground transition-colors hover:border-primary hover:text-foreground"
              >
                <PlugZap className="h-6 w-6" />
                <span className="text-sm">Create your first source</span>
              </button>
            )}
            {sources.map((s) => {
              const targetable = selection?.kind === "guardrail";
              return (
                <NodeCard
                  key={s.id}
                  nodeId={s.id}
                  title={s.name}
                  subtitle={`/${s.route}`}
                  icon={PlugZap}
                  href={`/dashboard/sources/${s.id}`}
                  enabled={s.enabled}
                  selected={selection?.kind === "source" && selection.id === s.id}
                  targetable={!!targetable}
                  attached={
                    targetable ? connected.has(connKey(s.id, selection!.id)) : false
                  }
                  registerRef={registerRef(s.id)}
                  onSelect={() =>
                    setSelection((sel) =>
                      sel?.kind === "source" && sel.id === s.id
                        ? null
                        : { kind: "source", id: s.id },
                    )
                  }
                  onToggleAttach={() => toggleAttach(s.id, selection!.id)}
                />
              );
            })}
          </div>

          {/* Guardrails column */}
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <h2 className="text-sm font-semibold text-muted-foreground">GUARDRAILS</h2>
              <Button
                size="sm"
                variant="outline"
                onClick={() => setGuardrailWizardOpen(true)}
              >
                <Plus className="mr-1 h-4 w-4" /> Add guardrail
              </Button>
            </div>
            {guardrails.length === 0 && (
              <button
                onClick={() => setGuardrailWizardOpen(true)}
                className="flex w-full flex-col items-center gap-2 rounded-lg border border-dashed p-8 text-muted-foreground transition-colors hover:border-primary hover:text-foreground"
              >
                <Shield className="h-6 w-6" />
                <span className="text-sm">Create your first guardrail</span>
              </button>
            )}
            {guardrails.map((g) => {
              const meta = guardTypeMeta(g.guard_type);
              const targetable = selection?.kind === "source";
              return (
                <NodeCard
                  key={g.id}
                  nodeId={g.id}
                  title={g.name}
                  subtitle={g.phase}
                  icon={meta?.icon ?? Shield}
                  badge={meta?.label ?? g.guard_type}
                  href={`/dashboard/guardrails/${g.id}`}
                  enabled={g.enabled}
                  selected={selection?.kind === "guardrail" && selection.id === g.id}
                  targetable={!!targetable}
                  attached={
                    targetable ? connected.has(connKey(selection!.id, g.id)) : false
                  }
                  registerRef={registerRef(g.id)}
                  onSelect={() =>
                    setSelection((sel) =>
                      sel?.kind === "guardrail" && sel.id === g.id
                        ? null
                        : { kind: "guardrail", id: g.id },
                    )
                  }
                  onToggleAttach={() => toggleAttach(selection!.id, g.id)}
                />
              );
            })}
          </div>
        </div>
        {selection && (
          <p className="mt-6 text-center text-sm text-muted-foreground">
            Click a card in the other column to attach or detach — Esc to cancel
          </p>
        )}
      </div>

      <AddSourceWizard
        open={sourceWizardOpen}
        onOpenChange={setSourceWizardOpen}
        onCreated={load}
      />
      <AddGuardrailWizard
        open={guardrailWizardOpen}
        onOpenChange={setGuardrailWizardOpen}
        onCreated={load}
      />
    </>
  );
}
