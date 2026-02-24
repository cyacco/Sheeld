"use client";

import type { Destination } from "@/lib/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

interface Props {
  destination: Destination;
  onToggle: (dest: Destination, enabled: boolean) => void;
  onEdit: (dest: Destination) => void;
  onDelete: (dest: Destination) => void;
}

export function DestinationCard({ destination, onToggle, onEdit, onDelete }: Props) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between">
          <CardTitle className="text-base">{destination.name}</CardTitle>
          <div className="flex items-center gap-2">
            <Badge variant="outline">{destination.guard_type}</Badge>
            <Badge variant="secondary">{destination.phase}</Badge>
          </div>
        </div>
      </CardHeader>
      <CardContent>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Switch
              checked={destination.enabled}
              onCheckedChange={(v) => onToggle(destination, v)}
            />
            <span className="text-sm text-muted-foreground">
              {destination.enabled ? "Enabled" : "Disabled"}
            </span>
          </div>
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={() => onEdit(destination)}>
              Edit
            </Button>
            <Button variant="destructive" size="sm" onClick={() => onDelete(destination)}>
              Delete
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
