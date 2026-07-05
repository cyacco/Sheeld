"use client";

import { useState, type ReactNode } from "react";
import { Check } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { cn } from "@/lib/utils";

export interface WizardStep {
  id: string;
  title: string;
  optional?: boolean;
  /** Returns an error message to block advancing, or null when valid. */
  validate?: () => string | null;
  render: () => ReactNode;
}

interface WizardProps {
  steps: WizardStep[];
  finishLabel: string;
  onFinish: () => Promise<void>;
  onCancel: () => void;
  /** Externally controlled step index (e.g. catalog tile click advances). */
  step: number;
  onStepChange: (step: number) => void;
}

export function Wizard({
  steps,
  finishLabel,
  onFinish,
  onCancel,
  step,
  onStepChange,
}: WizardProps) {
  const [submitting, setSubmitting] = useState(false);
  const current = steps[step];
  const isLast = step === steps.length - 1;

  function advance() {
    const error = current.validate?.();
    if (error) {
      toast.error(error);
      return;
    }
    if (!isLast) onStepChange(step + 1);
  }

  async function finish() {
    const error = current.validate?.();
    if (error) {
      toast.error(error);
      return;
    }
    setSubmitting(true);
    try {
      await onFinish();
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      {/* Stepper */}
      <div className="flex items-center gap-2 pb-4">
        {steps.map((s, i) => (
          <div key={s.id} className="flex items-center gap-2">
            <div
              className={cn(
                "flex h-6 w-6 items-center justify-center rounded-full text-xs font-medium",
                i < step
                  ? "bg-primary text-primary-foreground"
                  : i === step
                    ? "border-2 border-primary text-primary"
                    : "border border-muted-foreground/40 text-muted-foreground",
              )}
            >
              {i < step ? <Check className="h-3.5 w-3.5" /> : i + 1}
            </div>
            <span
              className={cn(
                "text-sm",
                i === step ? "font-medium" : "text-muted-foreground",
              )}
            >
              {s.title}
              {s.optional && (
                <span className="ml-1 text-xs text-muted-foreground">(optional)</span>
              )}
            </span>
            {i < steps.length - 1 && (
              <Separator className="w-6" orientation="horizontal" />
            )}
          </div>
        ))}
      </div>
      <Separator />

      {/* Body */}
      <div className="min-h-0 flex-1 overflow-y-auto py-4">{current.render()}</div>

      {/* Footer */}
      <Separator />
      <div className="flex items-center justify-between pt-4">
        <Button variant="ghost" onClick={onCancel} disabled={submitting}>
          Cancel
        </Button>
        <div className="flex gap-2">
          {step > 0 && (
            <Button
              variant="outline"
              onClick={() => onStepChange(step - 1)}
              disabled={submitting}
            >
              Back
            </Button>
          )}
          {isLast ? (
            <Button onClick={finish} disabled={submitting}>
              {submitting ? "Creating..." : finishLabel}
            </Button>
          ) : (
            <Button onClick={advance}>Next</Button>
          )}
        </div>
      </div>
    </div>
  );
}
