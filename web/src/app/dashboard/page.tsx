"use client";

import { ConnectionsBoard } from "@/components/connections/connections-board";

export default function ConnectionsPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">Connections</h1>
        <p className="text-sm text-muted-foreground">
          Wire sources to guardrails. Select a card, then click cards in the
          other column to attach or detach.
        </p>
      </div>
      <ConnectionsBoard />
    </div>
  );
}
