"use client";

import Link from "next/link";
import type { Source } from "@/lib/types";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

export function SourceCard({ source }: { source: Source }) {
  return (
    <Link href={`/dashboard/sources/${source.id}`}>
      <Card className="hover:border-primary/50 transition-colors cursor-pointer">
        <CardHeader className="pb-2">
          <div className="flex items-center justify-between">
            <CardTitle className="text-lg">{source.name}</CardTitle>
            <Badge variant={source.enabled ? "default" : "secondary"}>
              {source.enabled ? "Enabled" : "Disabled"}
            </Badge>
          </div>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground font-mono mb-1">/{source.slug}</p>
          {source.description && (
            <p className="text-sm text-muted-foreground mb-2">{source.description}</p>
          )}
          <div className="flex gap-2 text-xs text-muted-foreground">
            <span>{source.llm_provider}</span>
            <span>/</span>
            <span>{source.llm_model}</span>
          </div>
        </CardContent>
      </Card>
    </Link>
  );
}
