"use client";

import { Fragment, useCallback, useEffect, useState } from "react";
import type {
  AuditLog,
  GuardResultEntry,
  PhaseGuardResults,
  Source,
  TransformChainResult,
} from "@/lib/types";
import * as api from "@/lib/api";
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
import { toast } from "sonner";

const PAGE_SIZE = 25;

interface AuditLogTableProps {
  /** Fix the table to a single source (hides the Source column). */
  sourceId?: string;
  /** Source names for the Source column when not fixed. */
  sources?: Source[];
}

// Shared audit-log table with expandable guard-result rows and offset
// pagination. Used by the Audit Logs page and source detail Events tab.
export function AuditLogTable({ sourceId, sources }: AuditLogTableProps) {
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [offset, setOffset] = useState(0);
  const [expandedId, setExpandedId] = useState<string | null>(null);

  const loadLogs = useCallback(async () => {
    setLoading(true);
    try {
      const data = await api.listAuditLogs({
        limit: PAGE_SIZE,
        offset,
        source_id: sourceId,
      });
      setLogs(data ?? []);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load audit logs");
    } finally {
      setLoading(false);
    }
  }, [offset, sourceId]);

  useEffect(() => {
    loadLogs();
  }, [loadLogs]);

  // Reset pagination when the filter changes.
  useEffect(() => {
    setOffset(0);
  }, [sourceId]);

  function sourceName(id: string): string {
    const src = sources?.find((s) => s.id === id);
    return src?.name ?? id.slice(0, 8);
  }

  if (loading) {
    return (
      <div className="space-y-2">
        {[...Array(5)].map((_, i) => (
          <Skeleton key={i} className="h-10 w-full" />
        ))}
      </div>
    );
  }

  if (logs.length === 0) {
    return <p className="text-muted-foreground">No audit logs found.</p>;
  }

  const showSourceColumn = !sourceId;

  return (
    <>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead />
            <TableHead>Time</TableHead>
            {showSourceColumn && <TableHead>Source</TableHead>}
            <TableHead>Result</TableHead>
            <TableHead>Latency</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {logs.map((log) => (
            <Fragment key={log.id}>
              <TableRow
                className="cursor-pointer"
                onClick={() =>
                  setExpandedId(expandedId === log.id ? null : log.id)
                }
              >
                <TableCell className="w-8">
                  {expandedId === log.id ? "▼" : "▶"}
                </TableCell>
                <TableCell>{new Date(log.created_at).toLocaleString()}</TableCell>
                {showSourceColumn && (
                  <TableCell>{sourceName(log.source_id)}</TableCell>
                )}
                <TableCell>
                  <Badge
                    variant={
                      log.overall_result === "pass" ? "default" : "destructive"
                    }
                  >
                    {log.overall_result}
                  </Badge>
                </TableCell>
                <TableCell>{log.latency_ms}ms</TableCell>
              </TableRow>
              {expandedId === log.id && (
                <TableRow>
                  <TableCell
                    colSpan={showSourceColumn ? 5 : 4}
                    className="bg-muted/50"
                  >
                    <div className="space-y-3 p-2">
                      <TransformChainSection
                        title="Input transformations"
                        chain={log.guard_results?.transforms}
                      />
                      <GuardPhaseSection
                        title="Input guards"
                        result={log.guard_results?.input}
                      />
                      <TransformChainSection
                        title="Output transformations"
                        chain={log.guard_results?.output_transforms}
                      />
                      <GuardPhaseSection
                        title="Output guards"
                        result={log.guard_results?.output}
                      />
                      {!log.guard_results?.input &&
                        !log.guard_results?.output &&
                        !log.guard_results?.transforms &&
                        !log.guard_results?.output_transforms && (
                          <p className="text-sm text-muted-foreground">
                            No guards or transformations ran for this request.
                          </p>
                        )}
                    </div>
                  </TableCell>
                </TableRow>
              )}
            </Fragment>
          ))}
        </TableBody>
      </Table>

      <div className="mt-4 flex items-center justify-between">
        <Button
          variant="outline"
          size="sm"
          disabled={offset === 0}
          onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
        >
          Previous
        </Button>
        <span className="text-sm text-muted-foreground">
          Showing {offset + 1}–{offset + logs.length}
        </span>
        <Button
          variant="outline"
          size="sm"
          disabled={logs.length < PAGE_SIZE}
          onClick={() => setOffset(offset + PAGE_SIZE)}
        >
          Next
        </Button>
      </div>
    </>
  );
}

function GuardPhaseSection({
  title,
  result,
}: {
  title: string;
  result?: PhaseGuardResults;
}) {
  if (!result) return null;
  return (
    <div className="space-y-2">
      <h4 className="text-sm font-medium">
        {title} ({result.pass_count} passed, {result.fail_count} failed)
      </h4>
      {(result.results ?? []).map((gr: GuardResultEntry, i: number) => (
        <div key={i} className="flex items-center gap-3 text-sm">
          <Badge
            variant={gr.passed ? "default" : "destructive"}
            className="w-12 justify-center"
          >
            {gr.passed ? "pass" : "fail"}
          </Badge>
          {gr.shadow && (
            <Badge variant="outline" className="justify-center">
              shadow
            </Badge>
          )}
          <span className="font-mono">{gr.guard_name}</span>
          <span className="text-muted-foreground">({gr.guard_type})</span>
          <span className="text-muted-foreground">{gr.message}</span>
          <span className="ml-auto text-muted-foreground">{gr.duration_ms}ms</span>
        </div>
      ))}
    </div>
  );
}

function TransformChainSection({
  title,
  chain,
}: {
  title: string;
  chain?: TransformChainResult;
}) {
  if (!chain) return null;
  return (
    <div className="space-y-2">
      <h4 className="text-sm font-medium">
        {title} ({chain.changed ? "changed" : "no changes"},{" "}
        {chain.total_duration_ms}ms)
      </h4>
      {(chain.steps ?? []).map((step, i) => (
        <div key={i} className="flex items-center gap-3 text-sm">
          <Badge
            variant={step.errored ? "destructive" : step.changed ? "default" : "secondary"}
            className="w-20 justify-center"
          >
            {step.errored ? (step.skipped ? "skipped" : "errored") : step.changed ? "changed" : "no-op"}
          </Badge>
          <span className="font-mono">{step.name}</span>
          <span className="text-muted-foreground">({step.type})</span>
          {step.message && (
            <span className="text-muted-foreground">{step.message}</span>
          )}
          <span className="ml-auto text-muted-foreground">{step.duration_ms}ms</span>
        </div>
      ))}
    </div>
  );
}
