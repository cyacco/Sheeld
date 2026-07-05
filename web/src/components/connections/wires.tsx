"use client";

import { useCallback, useEffect, useState, type RefObject } from "react";

export interface Wire {
  key: string;
  fromId: string; // source node
  toId: string; // guardrail node
  highlighted: boolean;
  dashed: boolean;
}

interface WiresProps {
  containerRef: RefObject<HTMLDivElement | null>;
  nodeRefs: Map<string, HTMLElement>;
  wires: Wire[];
  /** Bump to force re-measure after data or layout changes. */
  version: number;
}

interface Path {
  key: string;
  d: string;
  highlighted: boolean;
  dashed: boolean;
}

// SVG overlay drawing bezier wires between source cards (right-center
// anchor) and guardrail cards (left-center anchor).
export function Wires({ containerRef, nodeRefs, wires, version }: WiresProps) {
  const [paths, setPaths] = useState<Path[]>([]);

  const measure = useCallback(() => {
    const container = containerRef.current;
    if (!container) return;
    const containerRect = container.getBoundingClientRect();

    const next: Path[] = [];
    for (const wire of wires) {
      const from = nodeRefs.get(wire.fromId);
      const to = nodeRefs.get(wire.toId);
      if (!from || !to) continue;
      const a = from.getBoundingClientRect();
      const b = to.getBoundingClientRect();
      const x1 = a.right - containerRect.left;
      const y1 = a.top + a.height / 2 - containerRect.top;
      const x2 = b.left - containerRect.left;
      const y2 = b.top + b.height / 2 - containerRect.top;
      next.push({
        key: wire.key,
        d: `M ${x1} ${y1} C ${x1 + 60} ${y1}, ${x2 - 60} ${y2}, ${x2} ${y2}`,
        highlighted: wire.highlighted,
        dashed: wire.dashed,
      });
    }
    setPaths(next);
  }, [containerRef, nodeRefs, wires]);

  useEffect(() => {
    measure();
    const container = containerRef.current;
    if (!container) return;
    const observer = new ResizeObserver(measure);
    observer.observe(container);
    window.addEventListener("resize", measure);
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", measure);
    };
  }, [measure, version]);

  return (
    <svg className="pointer-events-none absolute inset-0 h-full w-full">
      {paths.map((p) => (
        <path
          key={p.key}
          d={p.d}
          fill="none"
          stroke={p.highlighted ? "var(--primary)" : "var(--border)"}
          strokeWidth={p.highlighted ? 2 : 1.5}
          strokeDasharray={p.dashed ? "6 4" : undefined}
        />
      ))}
    </svg>
  );
}
