"use client";

import { useCallback, useEffect, useState } from "react";
import type { AlertWebhook } from "@/lib/types";
import * as api from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { toast } from "sonner";

export default function AlertsPage() {
  const [webhooks, setWebhooks] = useState<AlertWebhook[]>([]);
  const [loading, setLoading] = useState(true);
  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [format, setFormat] = useState("json");
  const [creating, setCreating] = useState(false);

  const load = useCallback(async () => {
    try {
      setWebhooks((await api.listAlertWebhooks()) ?? []);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load alerts");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setCreating(true);
    try {
      await api.createAlertWebhook({ name, url, payload_format: format });
      toast.success("Alert webhook created");
      setCreateOpen(false);
      setName("");
      setUrl("");
      setFormat("json");
      load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create alert webhook");
    } finally {
      setCreating(false);
    }
  }

  async function handleToggle(wh: AlertWebhook, enabled: boolean) {
    try {
      await api.updateAlertWebhook(wh.id, {
        name: wh.name,
        url: wh.url,
        payload_format: wh.payload_format,
        enabled,
      });
      load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to update alert webhook");
    }
  }

  async function handleDelete(id: string) {
    if (!confirm("Delete this alert webhook?")) return;
    try {
      await api.deleteAlertWebhook(id);
      toast.success("Alert webhook deleted");
      load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete alert webhook");
    }
  }

  return (
    <div>
      <div className="mb-2 flex items-center justify-between">
        <h2 className="text-2xl font-bold">Alerts</h2>
        <Dialog open={createOpen} onOpenChange={setCreateOpen}>
          <DialogTrigger asChild>
            <Button>Add Alert Webhook</Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Add Alert Webhook</DialogTitle>
            </DialogHeader>
            <form onSubmit={handleCreate} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="alertName">Name</Label>
                <Input
                  id="alertName"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="e.g. Security channel"
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="alertUrl">Webhook URL</Label>
                <Input
                  id="alertUrl"
                  type="url"
                  value={url}
                  onChange={(e) => setUrl(e.target.value)}
                  placeholder="https://hooks.slack.com/services/…"
                  required
                />
              </div>
              <div className="space-y-2">
                <Label>Payload format</Label>
                <Select value={format} onValueChange={setFormat}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="json">Generic JSON</SelectItem>
                    <SelectItem value="slack">Slack</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <Button type="submit" disabled={creating} className="w-full">
                {creating ? "Creating..." : "Create"}
              </Button>
            </form>
          </DialogContent>
        </Dialog>
      </div>

      <p className="mb-6 text-sm text-muted-foreground">
        Sheeld POSTs to these webhooks whenever a request is rejected by
        guards. Delivery is asynchronous and rate-capped per webhook; the
        audit log remains the complete record.
      </p>

      {loading ? (
        <div className="space-y-2">
          {[...Array(3)].map((_, i) => (
            <Skeleton key={i} className="h-10 w-full" />
          ))}
        </div>
      ) : webhooks.length === 0 ? (
        <p className="text-muted-foreground">
          No alert webhooks yet. Add one to get notified on guard rejections.
        </p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>URL</TableHead>
              <TableHead>Format</TableHead>
              <TableHead>Enabled</TableHead>
              <TableHead />
            </TableRow>
          </TableHeader>
          <TableBody>
            {webhooks.map((wh) => (
              <TableRow key={wh.id}>
                <TableCell className="font-medium">{wh.name}</TableCell>
                <TableCell className="max-w-md truncate font-mono text-sm">{wh.url}</TableCell>
                <TableCell>
                  <Badge variant="secondary">{wh.payload_format}</Badge>
                </TableCell>
                <TableCell>
                  <Switch
                    checked={wh.enabled}
                    onCheckedChange={(v) => handleToggle(wh, v)}
                  />
                </TableCell>
                <TableCell>
                  <Button
                    variant="destructive"
                    size="sm"
                    onClick={() => handleDelete(wh.id)}
                  >
                    Delete
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  );
}
