"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";
import type { Source } from "@/lib/types";
import { attachTransformer, createTransformer, listSources } from "@/lib/api";
import {
  emptyTransformerDraft,
  transformerDraftToParams,
  TransformerBasicsFields,
  TransformerConfigFields,
  type TransformerDraft,
} from "@/components/transformer-form";
import { TRANSFORMER_TYPES } from "@/components/transformer-type-meta";
import { Wizard, type WizardStep } from "@/components/wizards/wizard";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

interface AddTransformerWizardProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Pre-attach to this source and hide the attach step. */
  fixedSourceId?: string;
  onCreated: () => void;
}

export function AddTransformerWizard({
  open,
  onOpenChange,
  fixedSourceId,
  onCreated,
}: AddTransformerWizardProps) {
  const [step, setStep] = useState(0);
  const [draft, setDraft] = useState<TransformerDraft | null>(null);
  const [sources, setSources] = useState<Source[]>([]);
  const [attachIds, setAttachIds] = useState<Set<string>>(
    () => new Set(fixedSourceId ? [fixedSourceId] : []),
  );

  useEffect(() => {
    if (open && !fixedSourceId) {
      listSources().then(setSources).catch(() => setSources([]));
    }
  }, [open, fixedSourceId]);

  function handleOpenChange(next: boolean) {
    if (!next) {
      setStep(0);
      setDraft(null);
      setAttachIds(new Set(fixedSourceId ? [fixedSourceId] : []));
    }
    onOpenChange(next);
  }

  const steps: WizardStep[] = [
    {
      id: "type",
      title: "Transformation type",
      validate: () => (draft ? null : "Pick a transformation type"),
      render: () => (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          {TRANSFORMER_TYPES.map(({ value, label, description, icon: Icon }) => (
            <button
              key={value}
              type="button"
              onClick={() => {
                setDraft((d) =>
                  d?.transformerType === value ? d : emptyTransformerDraft(value),
                );
                setStep(1);
              }}
              className={cn(
                "flex items-start gap-3 rounded-lg border p-4 text-left transition-colors hover:bg-accent",
                draft?.transformerType === value &&
                  "border-primary ring-1 ring-primary",
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
      ),
    },
    {
      id: "configure",
      title: "Configure",
      validate: () => (draft?.name.trim() ? null : "Name is required"),
      render: () =>
        draft && (
          <div className="space-y-4">
            <TransformerBasicsFields draft={draft} onChange={setDraft} />
            <TransformerConfigFields draft={draft} onChange={setDraft} />
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
                  No sources yet — you can attach this transformation later
                  from a source&apos;s Transformations tab.
                </p>
              ) : (
                <div className="space-y-3">
                  <p className="text-sm text-muted-foreground">
                    Appends to the end of each source&apos;s transformation
                    chain; reorder from the source&apos;s Transformations tab.
                  </p>
                  {sources.map((s) => (
                    <div key={s.id} className="flex items-center gap-2">
                      <Checkbox
                        id={`attach-t-${s.id}`}
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
                      <Label htmlFor={`attach-t-${s.id}`} className="font-normal">
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
      const transformer = await createTransformer(transformerDraftToParams(draft));
      for (const sourceId of attachIds) {
        await attachTransformer(transformer.id, sourceId);
      }
      toast.success(`Transformation "${transformer.name}" created`);
      handleOpenChange(false);
      onCreated();
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : "Failed to create transformation",
      );
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="flex h-[85vh] max-w-3xl flex-col sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>Add transformation</DialogTitle>
        </DialogHeader>
        <Wizard
          steps={steps}
          step={step}
          onStepChange={setStep}
          finishLabel="Create transformation"
          onFinish={handleFinish}
          onCancel={() => handleOpenChange(false)}
        />
      </DialogContent>
    </Dialog>
  );
}
