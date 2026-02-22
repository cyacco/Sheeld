"use client";

import { useEffect, useState, useCallback } from "react";
import type { AuditLog, Source, GuardResultEntry } from "@/lib/types";
import * as api from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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

export default function AuditLogsPage() {
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [sources, setSources] = useState<Source[]>([]);
  const [loading, setLoading] = useState(true);
  const [sourceFilter, setSourceFilter] = useState<string>("all");
  const [offset, setOffset] = useState(0);
  const [expandedId, setExpandedId] = useState<string | null>(null);

  const loadLogs = useCallback(async () => {
    setLoading(true);
    try {
      const data = await api.listAuditLogs({
        limit: PAGE_SIZE,
        offset,
        source_id: sourceFilter !== "all" ? sourceFilter : undefined,
      });
      setLogs(data ?? []);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load audit logs");
    } finally {
      setLoading(false);
    }
  }, [offset, sourceFilter]);

  useEffect(() => {
    api.listSources().then((s) => setSources(s ?? [])).catch(() => {});
  }, []);

  useEffect(() => {
    loadLogs();
  }, [loadLogs]);

  function handleFilterChange(value: string) {
    setSourceFilter(value);
    setOffset(0);
  }

  function sourceName(sourceId: string): string {
    const src = sources.find((s) => s.id === sourceId);
    return src?.name ?? sourceId.slice(0, 8);
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold">Audit Logs</h2>
        <div className="flex items-center gap-2">
          <span className="text-sm text-muted-foreground">Source:</span>
          <Select value={sourceFilter} onValueChange={handleFilterChange}>
            <SelectTrigger className="w-48">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All Sources</SelectItem>
              {sources.map((src) => (
                <SelectItem key={src.id} value={src.id}>
                  {src.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      {loading ? (
        <p className="text-muted-foreground">Loading...</p>
      ) : logs.length === 0 ? (
        <p className="text-muted-foreground">No audit logs found.</p>
      ) : (
        <>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead />
                <TableHead>Time</TableHead>
                <TableHead>Source</TableHead>
                <TableHead>Result</TableHead>
                <TableHead>Latency</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {logs.map((log) => (
                <>
                  <TableRow
                    key={log.id}
                    className="cursor-pointer"
                    onClick={() =>
                      setExpandedId(expandedId === log.id ? null : log.id)
                    }
                  >
                    <TableCell className="w-8">
                      {expandedId === log.id ? "▼" : "▶"}
                    </TableCell>
                    <TableCell>
                      {new Date(log.created_at).toLocaleString()}
                    </TableCell>
                    <TableCell>{sourceName(log.source_id)}</TableCell>
                    <TableCell>
                      <Badge
                        variant={
                          log.overall_result === "pass"
                            ? "default"
                            : "destructive"
                        }
                      >
                        {log.overall_result}
                      </Badge>
                    </TableCell>
                    <TableCell>{log.latency_ms}ms</TableCell>
                  </TableRow>
                  {expandedId === log.id && (
                    <TableRow key={`${log.id}-detail`}>
                      <TableCell colSpan={5} className="bg-muted/50">
                        <div className="p-2 space-y-2">
                          <h4 className="text-sm font-medium">Guard Results</h4>
                          {(log.guard_results ?? []).map(
                            (gr: GuardResultEntry, i: number) => (
                              <div
                                key={i}
                                className="flex items-center gap-3 text-sm"
                              >
                                <Badge
                                  variant={gr.passed ? "default" : "destructive"}
                                  className="w-12 justify-center"
                                >
                                  {gr.passed ? "pass" : "fail"}
                                </Badge>
                                <span className="font-mono">{gr.guard_name}</span>
                                <span className="text-muted-foreground">
                                  ({gr.guard_type})
                                </span>
                                <span className="text-muted-foreground">
                                  {gr.message}
                                </span>
                                <span className="text-muted-foreground ml-auto">
                                  {gr.duration_ms}ms
                                </span>
                              </div>
                            ),
                          )}
                        </div>
                      </TableCell>
                    </TableRow>
                  )}
                </>
              ))}
            </TableBody>
          </Table>

          <div className="flex items-center justify-between mt-4">
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
      )}
    </div>
  );
}
