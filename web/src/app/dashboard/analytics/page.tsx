"use client";

import { useCallback, useEffect, useState } from "react";
import type { Analytics, DailyPoint, Source } from "@/lib/types";
import * as api from "@/lib/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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

const RANGES = [
  { value: "7", label: "Last 7 days" },
  { value: "30", label: "Last 30 days" },
  { value: "90", label: "Last 90 days" },
];

export default function AnalyticsPage() {
  const [days, setDays] = useState("30");
  const [data, setData] = useState<Analytics | null>(null);
  const [sources, setSources] = useState<Source[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [analytics, srcs] = await Promise.all([
        api.getAnalytics(Number(days)),
        api.listSources(),
      ]);
      setData(analytics);
      setSources(srcs ?? []);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load analytics");
    } finally {
      setLoading(false);
    }
  }, [days]);

  useEffect(() => {
    load();
  }, [load]);

  function sourceName(id: string): string {
    return sources.find((s) => s.id === id)?.name ?? id.slice(0, 8);
  }

  const passRate =
    data && data.summary.total_requests > 0
      ? Math.round((data.summary.passed / data.summary.total_requests) * 100)
      : null;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-2xl font-bold">Analytics</h2>
        <Select value={days} onValueChange={setDays}>
          <SelectTrigger className="w-40">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {RANGES.map((r) => (
              <SelectItem key={r.value} value={r.value}>
                {r.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {loading ? (
        <div className="space-y-4">
          <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
            {[...Array(4)].map((_, i) => (
              <Skeleton key={i} className="h-24 w-full" />
            ))}
          </div>
          <Skeleton className="h-56 w-full" />
        </div>
      ) : !data || data.summary.total_requests === 0 ? (
        <p className="text-muted-foreground">
          No requests in this window yet. Proxy some traffic to see usage here.
        </p>
      ) : (
        <>
          <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
            <StatTile label="Requests" value={data.summary.total_requests.toLocaleString()} />
            <StatTile
              label="Pass rate"
              value={passRate !== null ? `${passRate}%` : "—"}
              hint={`${data.summary.rejected.toLocaleString()} rejected`}
            />
            <StatTile
              label="Total tokens"
              value={data.summary.total_tokens.toLocaleString()}
              hint={`${data.summary.prompt_tokens.toLocaleString()} prompt / ${data.summary.completion_tokens.toLocaleString()} completion`}
            />
            <StatTile
              label="Est. cost"
              value={formatUSD(data.summary.estimated_cost_usd)}
              hint={
                data.summary.unpriced_requests > 0
                  ? `excludes ${data.summary.unpriced_requests.toLocaleString()} unpriced req`
                  : `prices as of ${data.prices_as_of}`
              }
            />
          </div>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">Tokens per day</CardTitle>
            </CardHeader>
            <CardContent>
              <UsageChart points={data.daily} />
            </CardContent>
          </Card>

          <div className="grid gap-4 lg:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle className="text-base">By model</CardTitle>
              </CardHeader>
              <CardContent>
                {data.by_model.length === 0 ? (
                  <p className="text-sm text-muted-foreground">No model data.</p>
                ) : (
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>Model</TableHead>
                        <TableHead className="text-right">Requests</TableHead>
                        <TableHead className="text-right">Tokens</TableHead>
                        <TableHead className="text-right">Est. cost</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {data.by_model.map((m) => (
                        <TableRow key={m.model}>
                          <TableCell className="font-mono text-sm">{m.model}</TableCell>
                          <TableCell className="text-right">{m.requests.toLocaleString()}</TableCell>
                          <TableCell className="text-right">{m.total_tokens.toLocaleString()}</TableCell>
                          <TableCell className="text-right text-muted-foreground">
                            {m.estimated_cost_usd != null ? formatUSD(m.estimated_cost_usd) : "—"}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                )}
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-base">By source</CardTitle>
              </CardHeader>
              <CardContent>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Source</TableHead>
                      <TableHead className="text-right">Requests</TableHead>
                      <TableHead className="text-right">Rejected</TableHead>
                      <TableHead className="text-right">Tokens</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {data.by_source.map((s) => (
                      <TableRow key={s.source_id}>
                        <TableCell>{sourceName(s.source_id)}</TableCell>
                        <TableCell className="text-right">{s.requests.toLocaleString()}</TableCell>
                        <TableCell className="text-right">{s.rejected.toLocaleString()}</TableCell>
                        <TableCell className="text-right">{s.total_tokens.toLocaleString()}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </CardContent>
            </Card>
          </div>
        </>
      )}
    </div>
  );
}

// formatUSD shows small estimated costs with enough precision to be useful
// (sub-cent totals are common in testing) while keeping larger ones readable.
function formatUSD(v: number): string {
  if (v === 0) return "$0";
  const digits = v < 1 ? 4 : 2;
  return `$${v.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: digits })}`;
}

function StatTile({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <Card>
      <CardContent className="pt-6">
        <p className="text-sm text-muted-foreground">{label}</p>
        <p className="mt-1 text-2xl font-semibold tabular-nums">{value}</p>
        {hint && <p className="mt-1 text-xs text-muted-foreground">{hint}</p>}
      </CardContent>
    </Card>
  );
}

// UsageChart is a minimal inline-SVG bar chart of daily token totals. It uses
// the app's foreground/primary tokens so it inherits the active theme.
function UsageChart({ points }: { points: DailyPoint[] }) {
  if (points.length === 0) {
    return <p className="text-sm text-muted-foreground">No usage in this window.</p>;
  }
  const H = 160;
  const pad = 4;
  const max = Math.max(1, ...points.map((p) => p.total_tokens));
  const barGap = 3;
  // Cap the slot width so a handful of days render as distinct bars rather
  // than one page-wide block; the SVG width grows with the data instead.
  const slot = Math.min(56, Math.max(8, 640 / points.length));
  const barW = slot - barGap;
  const W = Math.max(320, points.length * slot + pad * 2);

  return (
    <div className="w-full overflow-x-auto">
      <svg
        viewBox={`0 0 ${W} ${H}`}
        width={W}
        height={H}
        className="h-40 max-w-full"
        role="img"
        aria-label="Tokens per day"
      >
        {points.map((p, i) => {
          const h = (p.total_tokens / max) * (H - pad * 2);
          const x = pad + i * slot;
          const y = H - pad - h;
          return (
            <rect key={p.day} x={x} y={y} width={barW} height={h} rx={2} className="fill-primary">
              <title>{`${p.day}: ${p.total_tokens.toLocaleString()} tokens, ${p.requests.toLocaleString()} requests`}</title>
            </rect>
          );
        })}
      </svg>
      <div className="mt-2 flex justify-between text-xs text-muted-foreground">
        <span>{points[0].day}</span>
        <span>{points[points.length - 1].day}</span>
      </div>
    </div>
  );
}
