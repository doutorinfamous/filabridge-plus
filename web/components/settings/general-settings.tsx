"use client";

import * as React from "react";
import { Home, Loader2, PlugZap, Save } from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

function Field({
  id,
  label,
  hint,
  children,
}: {
  id: string;
  label: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      {children}
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  );
}

export function SpoolmanSettings() {
  const [loading, setLoading] = React.useState(true);
  const [saving, setSaving] = React.useState(false);
  const [testing, setTesting] = React.useState(false);
  const [form, setForm] = React.useState({
    spoolman_url: "",
    spoolman_username: "",
    spoolman_password: "",
  });

  React.useEffect(() => {
    api
      .getConfig()
      .then((cfg) => {
        setForm({
          spoolman_url: cfg.spoolman_url ?? "",
          spoolman_username: cfg.spoolman_username ?? "",
          spoolman_password: cfg.spoolman_password ?? "",
        });
      })
      .catch(() => toast.error("Failed to load configuration"))
      .finally(() => setLoading(false));
  }, []);

  const save = async () => {
    setSaving(true);
    try {
      await api.updateConfig(form);
      toast.success("Configuration saved");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to save");
    } finally {
      setSaving(false);
    }
  };

  const test = async () => {
    if (!form.spoolman_url.trim()) {
      toast.error("Enter the Spoolman URL first");
      return;
    }
    setTesting(true);
    try {
      // Tests the values currently in the form, even before saving
      await api.testSpoolman({
        spoolman_url: form.spoolman_url.trim(),
        spoolman_username: form.spoolman_username,
        spoolman_password: form.spoolman_password,
      });
      toast.success("Spoolman connection OK");
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Connection failed"
      );
    } finally {
      setTesting(false);
    }
  };

  return (
    <Card className="border-border/70 bg-card/60">
      <CardHeader>
        <CardTitle className="text-base">Spoolman</CardTitle>
        <CardDescription>
          Filament inventory used to debit print consumption — required for
          FilaBridge+ to work
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <Field
          id="spoolman_url"
          label="Spoolman URL"
          hint="e.g. http://192.168.1.10:8000 (use host.docker.internal inside Docker)"
        >
          <Input
            id="spoolman_url"
            value={form.spoolman_url}
            disabled={loading}
            onChange={(e) =>
              setForm((f) => ({ ...f, spoolman_url: e.target.value }))
            }
            placeholder="http://localhost:8000"
          />
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field id="spoolman_username" label="Username (optional)">
            <Input
              id="spoolman_username"
              value={form.spoolman_username}
              disabled={loading}
              onChange={(e) =>
                setForm((f) => ({ ...f, spoolman_username: e.target.value }))
              }
              placeholder="Basic auth"
            />
          </Field>
          <Field id="spoolman_password" label="Password (optional)">
            <Input
              id="spoolman_password"
              type="password"
              value={form.spoolman_password}
              disabled={loading}
              onChange={(e) =>
                setForm((f) => ({ ...f, spoolman_password: e.target.value }))
              }
              placeholder="Basic auth"
            />
          </Field>
        </div>
        <div className="flex flex-wrap gap-2 pt-1">
          <Button onClick={save} disabled={saving || loading}>
            {saving ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <Save className="size-4" />
            )}
            Save
          </Button>
          <Button variant="outline" onClick={test} disabled={testing}>
            {testing ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <PlugZap className="size-4" />
            )}
            Test connection
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

export function HomeAssistantSettings() {
  const [loading, setLoading] = React.useState(true);
  const [saving, setSaving] = React.useState(false);
  const [testing, setTesting] = React.useState(false);
  const [tokenSet, setTokenSet] = React.useState(false);
  const [form, setForm] = React.useState({
    ha_url: "",
    ha_token: "",
    filabridge_public_url: "",
  });

  React.useEffect(() => {
    api
      .getHAConfig()
      .then((cfg) => {
        setForm((f) => ({
          ...f,
          ha_url: cfg.ha_url ?? "",
          filabridge_public_url: cfg.filabridge_public_url ?? "",
        }));
        setTokenSet(cfg.ha_token_set);
      })
      .catch(() => undefined)
      .finally(() => setLoading(false));
  }, []);

  const save = async () => {
    if (!form.ha_url.trim()) {
      toast.error("Enter the Home Assistant URL");
      return;
    }
    setSaving(true);
    try {
      await api.updateHAConfig({
        ha_url: form.ha_url.trim(),
        filabridge_public_url: form.filabridge_public_url.trim(),
        ...(form.ha_token.trim() ? { ha_token: form.ha_token.trim() } : {}),
      });
      toast.success("Home Assistant configuration saved");
      if (form.ha_token.trim()) setTokenSet(true);
      setForm((f) => ({ ...f, ha_token: "" }));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to save");
    } finally {
      setSaving(false);
    }
  };

  const test = async () => {
    setTesting(true);
    try {
      await api.testHA(form.ha_url.trim(), form.ha_token.trim());
      toast.success("Home Assistant connection OK");
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "HA connection failed"
      );
    } finally {
      setTesting(false);
    }
  };

  return (
    <Card className="border-border/70 bg-card/60">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Home className="size-4" /> Home Assistant (Bambu Lab)
        </CardTitle>
        <CardDescription>
          Connect to Home Assistant with the ha-bambulab integration for
          automatic filament tracking
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <Field id="ha_url" label="Home Assistant URL">
          <Input
            id="ha_url"
            value={form.ha_url}
            disabled={loading}
            onChange={(e) => setForm((f) => ({ ...f, ha_url: e.target.value }))}
            placeholder="http://192.168.1.10:8123"
          />
        </Field>
        <Field
          id="ha_token"
          label={`Access token ${tokenSet ? "(configured)" : "(not set)"}`}
          hint="Long-Lived Access Token — leave blank to keep the current token"
        >
          <Input
            id="ha_token"
            type="password"
            value={form.ha_token}
            disabled={loading}
            onChange={(e) =>
              setForm((f) => ({ ...f, ha_token: e.target.value }))
            }
            placeholder="Long-Lived Access Token"
          />
        </Field>
        <Field
          id="filabridge_public_url"
          label="FilaBridge+ public URL (webhooks and NFC/QR tags)"
          hint="Used in HA webhooks and as the base for NFC/QR tag URLs — must be reachable on your network (do not use localhost or 0.0.0.0)"
        >
          <Input
            id="filabridge_public_url"
            value={form.filabridge_public_url}
            disabled={loading}
            onChange={(e) =>
              setForm((f) => ({ ...f, filabridge_public_url: e.target.value }))
            }
            placeholder="http://192.168.1.20:5000"
          />
        </Field>
        <div className="flex flex-wrap gap-2 pt-1">
          <Button onClick={save} disabled={saving || loading}>
            {saving ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <Save className="size-4" />
            )}
            Save
          </Button>
          <Button variant="outline" onClick={test} disabled={testing}>
            {testing ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <PlugZap className="size-4" />
            )}
            Test connection
          </Button>
        </div>
        <p className="text-xs text-muted-foreground">
          Full guide: <code>docs/home-assistant-setup.md</code> in the
          FilaBridge+ repository.
        </p>
      </CardContent>
    </Card>
  );
}
