"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";
import type { Guardrail } from "@/lib/types";
import { attachGuardrail, createSource, listGuardrails } from "@/lib/api";
import {
  emptySourceDraft,
  sourceDraftToParams,
  SourceBasicsFields,
  SourceLLMFields,
  type SourceDraft,
} from "@/components/source-form";
import { guardTypeMeta } from "@/components/guard-type-meta";
import { Wizard, type WizardStep } from "@/components/wizards/wizard";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

interface AddSourceWizardProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: () => void;
}

export function AddSourceWizard({ open, onOpenChange, onCreated }: AddSourceWizardProps) {
  const [step, setStep] = useState(0);
  const [draft, setDraft] = useState<SourceDraft>(emptySourceDraft);
  const [guardrails, setGuardrails] = useState<Guardrail[]>([]);
  const [attachIds, setAttachIds] = useState<Set<string>>(new Set());

  // Load guardrails for the attach step when opening; reset state on close
  // so the next open starts fresh.
  useEffect(() => {
    if (open) {
      listGuardrails().then(setGuardrails).catch(() => setGuardrails([]));
    }
  }, [open]);

  function handleOpenChange(next: boolean) {
    if (!next) {
      setStep(0);
      setDraft(emptySourceDraft());
      setAttachIds(new Set());
    }
    onOpenChange(next);
  }

  const steps: WizardStep[] = [
    {
      id: "basics",
      title: "Basics",
      validate: () => {
        if (!draft.name.trim()) return "Name is required";
        if (!draft.route.trim()) return "Route is required";
        return null;
      },
      render: () => <SourceBasicsFields draft={draft} onChange={setDraft} />,
    },
    {
      id: "llm",
      title: "LLM",
      validate: () => {
        if (!draft.llmModel) return "Model is required";
        if (!draft.llmApiKey) return "LLM API key is required";
        if (draft.passCriteria === "n_of_m" && !draft.passThreshold)
          return "Threshold is required for n-of-m criteria";
        return null;
      },
      render: () => <SourceLLMFields draft={draft} onChange={setDraft} />,
    },
    {
      id: "attach",
      title: "Attach guardrails",
      optional: true,
      render: () =>
        guardrails.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No guardrails yet — you can attach them later from the Connections
            page.
          </p>
        ) : (
          <div className="space-y-3">
            {guardrails.map((g) => (
              <div key={g.id} className="flex items-center gap-2">
                <Checkbox
                  id={`attach-${g.id}`}
                  checked={attachIds.has(g.id)}
                  onCheckedChange={(checked) => {
                    setAttachIds((prev) => {
                      const next = new Set(prev);
                      if (checked) next.add(g.id);
                      else next.delete(g.id);
                      return next;
                    });
                  }}
                />
                <Label htmlFor={`attach-${g.id}`} className="font-normal">
                  {g.name}
                </Label>
                <Badge variant="secondary">
                  {guardTypeMeta(g.guard_type)?.label ?? g.guard_type}
                </Badge>
                <Badge variant="outline">{g.phase}</Badge>
              </div>
            ))}
          </div>
        ),
    },
  ];

  async function handleFinish() {
    try {
      const source = await createSource(sourceDraftToParams(draft));
      for (const guardrailId of attachIds) {
        await attachGuardrail(guardrailId, source.id);
      }
      toast.success(`Source "${source.name}" created`);
      handleOpenChange(false);
      onCreated();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create source");
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="flex h-[85vh] max-w-3xl flex-col sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>Add source</DialogTitle>
        </DialogHeader>
        <Wizard
          steps={steps}
          step={step}
          onStepChange={setStep}
          finishLabel="Create source"
          onFinish={handleFinish}
          onCancel={() => handleOpenChange(false)}
        />
      </DialogContent>
    </Dialog>
  );
}
