"use client";

import { GUARD_TYPES } from "@/components/guard-type-meta";
import { cn } from "@/lib/utils";

interface GuardrailCatalogProps {
  selected: string | null;
  onSelect: (guardType: string) => void;
}

// Catalog tile grid for picking a guard type (rudderstack-style).
export function GuardrailCatalog({ selected, onSelect }: GuardrailCatalogProps) {
  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
      {GUARD_TYPES.map(({ value, label, description, icon: Icon }) => (
        <button
          key={value}
          type="button"
          onClick={() => onSelect(value)}
          className={cn(
            "flex items-start gap-3 rounded-lg border p-4 text-left transition-colors hover:bg-accent",
            selected === value && "border-primary ring-1 ring-primary",
          )}
        >
          <div className="rounded-md bg-primary/10 p-2 text-primary">
            <Icon className="h-5 w-5" />
          </div>
          <div>
            <div className="font-medium">{label}</div>
            <div className="text-sm text-muted-foreground">{description}</div>
          </div>
        </button>
      ))}
    </div>
  );
}
