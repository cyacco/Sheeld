"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";
import type { Source } from "@/lib/types";
import { attachGuardrail, createGuardrail, listSources } from "@/lib/api";
import {
  emptyGuardrailDraft,
  guardrailDraftToParams,
  GuardrailBasicsFields,
  GuardConfigFields,
  type GuardrailDraft,
} from "@/components/guardrail-form";
import { GuardrailCatalog } from "@/components/wizards/guardrail-catalog";
import { Wizard, type WizardStep } from "@/components/wizards/wizard";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

interface AddGuardrailWizardProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Pre-attach to this source and hide the attach step. */
  fixedSourceId?: string;
  onCreated: () => void;
}

export function AddGuardrailWizard({
  open,
  onOpenChange,
  fixedSourceId,
  onCreated,
}: AddGuardrailWizardProps) {
  const [step, setStep] = useState(0);
  const [draft, setDraft] = useState<GuardrailDraft | null>(null);
  const [sources, setSources] = useState<Source[]>([]);
  const [attachIds, setAttachIds] = useState<Set<string>>(new Set());

  // Reset per open; load sources for the attach step.
  useEffect(() => {
    if (!open) return;
    setStep(0);
    setDraft(null);
    setAttachIds(new Set(fixedSourceId ? [fixedSourceId] : []));
    if (!fixedSourceId) {
      listSources().then(setSources).catch(() => setSources([]));
    }
  }, [open, fixedSourceId]);

  const steps: WizardStep[] = [
    {
      id: "type",
      title: "Guard type",
      validate: () => (draft ? null : "Pick a guard type"),
      render: () => (
        <GuardrailCatalog
          selected={draft?.guardType ?? null}
          onSelect={(guardType) => {
            setDraft((d) =>
              d?.guardType === guardType ? d : emptyGuardrailDraft(guardType),
            );
            setStep(1);
          }}
        />
      ),
    },
    {
      id: "configure",
      title: "Configure",
      validate: () => (draft?.name.trim() ? null : "Name is required"),
      render: () =>
        draft && (
          <div className="space-y-4">
            <GuardrailBasicsFields draft={draft} onChange={setDraft} />
            <GuardConfigFields draft={draft} onChange={setDraft} />
          </div>
        ),
    },
    ...(fixedSourceId
      ? []
      : [
          {
            id: "attach",
            title: "Attach to sources",
            optional: true,
            render: () =>
              sources.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  No sources yet — you can attach this guardrail later from the
                  Connections page.
                </p>
              ) : (
                <div className="space-y-3">
                  {sources.map((s) => (
                    <div key={s.id} className="flex items-center gap-2">
                      <Checkbox
                        id={`attach-${s.id}`}
                        checked={attachIds.has(s.id)}
                        onCheckedChange={(checked) => {
                          setAttachIds((prev) => {
                            const next = new Set(prev);
                            if (checked) next.add(s.id);
                            else next.delete(s.id);
                            return next;
                          });
                        }}
                      />
                      <Label htmlFor={`attach-${s.id}`} className="font-normal">
                        {s.name}{" "}
                        <span className="text-muted-foreground">/{s.route}</span>
                      </Label>
                    </div>
                  ))}
                </div>
              ),
          } satisfies WizardStep,
        ]),
  ];

  async function handleFinish() {
    if (!draft) return;
    try {
      const guardrail = await createGuardrail(guardrailDraftToParams(draft));
      for (const sourceId of attachIds) {
        await attachGuardrail(guardrail.id, sourceId);
      }
      toast.success(`Guardrail "${guardrail.name}" created`);
      onOpenChange(false);
      onCreated();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create guardrail");
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex h-[85vh] max-w-3xl flex-col sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>Add guardrail</DialogTitle>
        </DialogHeader>
        <Wizard
          steps={steps}
          step={step}
          onStepChange={setStep}
          finishLabel="Create guardrail"
          onFinish={handleFinish}
          onCancel={() => onOpenChange(false)}
        />
      </DialogContent>
    </Dialog>
  );
}
