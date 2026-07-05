"use client";

import { useEffect, useState } from "react";
import type { Source } from "@/lib/types";
import * as api from "@/lib/api";
import { AuditLogTable } from "@/components/audit-log-table";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

export default function AuditLogsPage() {
  const [sources, setSources] = useState<Source[]>([]);
  const [sourceFilter, setSourceFilter] = useState<string>("all");

  useEffect(() => {
    api.listSources().then((s) => setSources(s ?? [])).catch(() => {});
  }, []);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Audit Logs</h1>
          <p className="text-sm text-muted-foreground">
            Request history with per-guard results.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-sm text-muted-foreground">Source:</span>
          <Select value={sourceFilter} onValueChange={setSourceFilter}>
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

      <AuditLogTable
        sourceId={sourceFilter !== "all" ? sourceFilter : undefined}
        sources={sources}
      />
    </div>
  );
}
