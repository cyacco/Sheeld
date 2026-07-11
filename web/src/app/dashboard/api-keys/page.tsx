"use client";

import { useEffect, useState } from "react";
import type { APIKey } from "@/lib/types";
import * as api from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { toast } from "sonner";

export default function APIKeysPage() {
  const [keys, setKeys] = useState<APIKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState("");
  const [rps, setRps] = useState("");
  const [burst, setBurst] = useState("");
  const [creating, setCreating] = useState(false);
  const [rawKey, setRawKey] = useState<string | null>(null);

  useEffect(() => {
    loadKeys();
  }, []);

  async function loadKeys() {
    try {
      const data = await api.listAPIKeys();
      setKeys(data ?? []);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load API keys");
    } finally {
      setLoading(false);
    }
  }

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setCreating(true);
    try {
      const result = await api.createAPIKey(name, {
        rps: rps ? Number(rps) : undefined,
        burst: burst ? Number(burst) : undefined,
      });
      setRawKey(result.raw_key);
      setName("");
      setRps("");
      setBurst("");
      loadKeys();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create API key");
    } finally {
      setCreating(false);
    }
  }

  async function handleRevoke(keyId: string) {
    if (!confirm("Revoke this API key? This cannot be undone.")) return;
    try {
      await api.revokeAPIKey(keyId);
      toast.success("API key revoked");
      loadKeys();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to revoke API key");
    }
  }

  function handleCopy() {
    if (rawKey) {
      navigator.clipboard.writeText(rawKey);
      toast.success("Copied to clipboard");
    }
  }

  function handleCloseCreate() {
    setCreateOpen(false);
    setRawKey(null);
    setName("");
    setRps("");
    setBurst("");
  }

  if (loading) return <p className="text-muted-foreground">Loading API keys...</p>;

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold">API Keys</h2>
        <Dialog open={createOpen} onOpenChange={(open) => (open ? setCreateOpen(true) : handleCloseCreate())}>
          <DialogTrigger asChild>
            <Button>Create API Key</Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>
                {rawKey ? "API Key Created" : "Create API Key"}
              </DialogTitle>
            </DialogHeader>
            {rawKey ? (
              <div className="space-y-4">
                <p className="text-sm text-muted-foreground">
                  Copy this key now. You won&apos;t be able to see it again.
                </p>
                <div className="flex gap-2">
                  <Input value={rawKey} readOnly className="font-mono text-sm" />
                  <Button onClick={handleCopy} variant="outline">
                    Copy
                  </Button>
                </div>
                <Button onClick={handleCloseCreate} className="w-full">
                  Done
                </Button>
              </div>
            ) : (
              <form onSubmit={handleCreate} className="space-y-4">
                <div className="space-y-2">
                  <Label htmlFor="keyName">Key Name</Label>
                  <Input
                    id="keyName"
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder="e.g. Production"
                    required
                  />
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <div className="space-y-2">
                    <Label htmlFor="keyRps">Rate limit (req/s)</Label>
                    <Input
                      id="keyRps"
                      type="number"
                      min={0}
                      step="any"
                      value={rps}
                      onChange={(e) => setRps(e.target.value)}
                      placeholder="Default"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="keyBurst">Burst</Label>
                    <Input
                      id="keyBurst"
                      type="number"
                      min={0}
                      value={burst}
                      onChange={(e) => setBurst(e.target.value)}
                      placeholder="Default"
                    />
                  </div>
                </div>
                <p className="text-xs text-muted-foreground">
                  Leave blank to use the data plane&apos;s default rate limit.
                  Applies per key.
                </p>
                <Button type="submit" disabled={creating} className="w-full">
                  {creating ? "Creating..." : "Create"}
                </Button>
              </form>
            )}
          </DialogContent>
        </Dialog>
      </div>

      {keys.length === 0 ? (
        <p className="text-muted-foreground">No API keys yet.</p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Prefix</TableHead>
              <TableHead>Rate limit</TableHead>
              <TableHead>Created</TableHead>
              <TableHead>Status</TableHead>
              <TableHead />
            </TableRow>
          </TableHeader>
          <TableBody>
            {keys.map((key) => (
              <TableRow key={key.id}>
                <TableCell className="font-medium">{key.name}</TableCell>
                <TableCell className="font-mono text-sm">{key.key_prefix}...</TableCell>
                <TableCell className="text-sm text-muted-foreground">
                  {key.rate_limit_rps
                    ? `${key.rate_limit_rps}/s${key.rate_limit_burst ? ` · burst ${key.rate_limit_burst}` : ""}`
                    : "Default"}
                </TableCell>
                <TableCell>{new Date(key.created_at).toLocaleDateString()}</TableCell>
                <TableCell>
                  {key.revoked_at ? (
                    <Badge variant="destructive">Revoked</Badge>
                  ) : (
                    <Badge variant="default">Active</Badge>
                  )}
                </TableCell>
                <TableCell>
                  {!key.revoked_at && (
                    <Button
                      variant="destructive"
                      size="sm"
                      onClick={() => handleRevoke(key.id)}
                    >
                      Revoke
                    </Button>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  );
}
