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
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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

// A keyset cursor pointing at the last row of a page.
interface Cursor {
  before: string;
  before_id: string;
}

// Shared audit-log table with expandable guard-result rows, status/date
// filters, and keyset pagination. Used by the Audit Logs page and the source
// detail Events tab.
export function AuditLogTable({ sourceId, sources }: AuditLogTableProps) {
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [expandedId, setExpandedId] = useState<string | null>(null);

  // Filters.
  const [status, setStatus] = useState("all");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");

  // Cursor stack: empty = first page; each entry is the cursor used to fetch
  // the page at that depth, so we can walk back with "Previous".
  const [cursors, setCursors] = useState<Cursor[]>([]);

  const loadLogs = useCallback(async () => {
    setLoading(true);
    try {
      const cursor = cursors[cursors.length - 1];
      const data = await api.listAuditLogs({
        limit: PAGE_SIZE,
        source_id: sourceId,
        status: status === "all" ? undefined : status,
        from: from ? `${from}T00:00:00Z` : undefined,
        to: to ? `${to}T23:59:59Z` : undefined,
        before: cursor?.before,
        before_id: cursor?.before_id,
      });
      setLogs(data ?? []);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load audit logs");
    } finally {
      setLoading(false);
    }
  }, [cursors, sourceId, status, from, to]);

  useEffect(() => {
    loadLogs();
  }, [loadLogs]);

  // Reset to the first page whenever a filter changes.
  useEffect(() => {
    setCursors([]);
    setExpandedId(null);
  }, [sourceId, status, from, to]);

  function nextPage() {
    const last = logs[logs.length - 1];
    if (!last) return;
    setCursors((c) => [...c, { before: last.created_at, before_id: last.id }]);
    setExpandedId(null);
  }

  function prevPage() {
    setCursors((c) => c.slice(0, -1));
    setExpandedId(null);
  }

  function sourceName(id: string): string {
    const src = sources?.find((s) => s.id === id);
    return src?.name ?? id.slice(0, 8);
  }

  const showSourceColumn = !sourceId;
  const page = cursors.length + 1;

  return (
    <>
      <div className="mb-4 flex flex-wrap items-end gap-3">
        <div className="space-y-1">
          <label className="text-xs text-muted-foreground">Result</label>
          <Select value={status} onValueChange={setStatus}>
            <SelectTrigger className="w-32">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All</SelectItem>
              <SelectItem value="pass">Pass</SelectItem>
              <SelectItem value="fail">Fail</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1">
          <label className="text-xs text-muted-foreground">From</label>
          <Input
            type="date"
            value={from}
            max={to || undefined}
            onChange={(e) => setFrom(e.target.value)}
            className="w-40"
          />
        </div>
        <div className="space-y-1">
          <label className="text-xs text-muted-foreground">To</label>
          <Input
            type="date"
            value={to}
            min={from || undefined}
            onChange={(e) => setTo(e.target.value)}
            className="w-40"
          />
        </div>
        {(status !== "all" || from || to) && (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              setStatus("all");
              setFrom("");
              setTo("");
            }}
          >
            Clear
          </Button>
        )}
      </div>

      {loading ? (
        <div className="space-y-2">
          {[...Array(5)].map((_, i) => (
            <Skeleton key={i} className="h-10 w-full" />
          ))}
        </div>
      ) : logs.length === 0 ? (
        <p className="text-muted-foreground">No audit logs found.</p>
      ) : (
        <>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead />
            <TableHead>Time</TableHead>
            {showSourceColumn && <TableHead>Source</TableHead>}
            <TableHead>Result</TableHead>
            <TableHead>Tokens</TableHead>
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
                <TableCell className="text-muted-foreground">
                  {log.total_tokens != null ? (
                    <span title={`${log.prompt_tokens ?? 0} prompt + ${log.completion_tokens ?? 0} completion`}>
                      {log.total_tokens}
                      {log.model ? ` · ${log.model}` : ""}
                    </span>
                  ) : (
                    "—"
                  )}
                </TableCell>
                <TableCell>{log.latency_ms}ms</TableCell>
              </TableRow>
              {expandedId === log.id && (
                <TableRow>
                  <TableCell
                    colSpan={showSourceColumn ? 6 : 5}
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
          disabled={cursors.length === 0}
          onClick={prevPage}
        >
          Previous
        </Button>
        <span className="text-sm text-muted-foreground">Page {page}</span>
        <Button
          variant="outline"
          size="sm"
          disabled={logs.length < PAGE_SIZE}
          onClick={nextPage}
        >
          Next
        </Button>
      </div>
        </>
      )}
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
