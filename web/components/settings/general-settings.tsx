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
    poll_interval: "30",
  });

  React.useEffect(() => {
    api
      .getConfig()
      .then((cfg) => {
        setForm({
          spoolman_url: cfg.spoolman_url ?? "",
          spoolman_username: cfg.spoolman_username ?? "",
          spoolman_password: cfg.spoolman_password ?? "",
          poll_interval: cfg.poll_interval ?? "30",
        });
      })
      .catch(() => toast.error("Falha ao carregar configuração"))
      .finally(() => setLoading(false));
  }, []);

  const save = async () => {
    setSaving(true);
    try {
      await api.updateConfig(form);
      toast.success("Configuração salva");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Falha ao salvar");
    } finally {
      setSaving(false);
    }
  };

  const test = async () => {
    setTesting(true);
    try {
      await api.testSpoolman();
      toast.success("Conexão com o Spoolman OK");
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Falha na conexão"
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
          Inventário de filamento usado para debitar o consumo das impressões
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <Field
          id="spoolman_url"
          label="URL do Spoolman"
          hint="Ex.: http://192.168.1.10:8000 (use host.docker.internal dentro do Docker)"
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
          <Field id="spoolman_username" label="Usuário (opcional)">
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
          <Field id="spoolman_password" label="Senha (opcional)">
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
        <Field
          id="poll_interval"
          label="Intervalo de polling (segundos)"
          hint="Frequência de verificação do status das impressoras Moonraker"
        >
          <Input
            id="poll_interval"
            type="number"
            min={10}
            max={300}
            value={form.poll_interval}
            disabled={loading}
            onChange={(e) =>
              setForm((f) => ({ ...f, poll_interval: e.target.value }))
            }
            className="max-w-40"
          />
        </Field>
        <div className="flex flex-wrap gap-2 pt-1">
          <Button onClick={save} disabled={saving || loading}>
            {saving ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <Save className="size-4" />
            )}
            Salvar
          </Button>
          <Button variant="outline" onClick={test} disabled={testing}>
            {testing ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <PlugZap className="size-4" />
            )}
            Testar conexão
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
      toast.error("Informe a URL do Home Assistant");
      return;
    }
    setSaving(true);
    try {
      await api.updateHAConfig({
        ha_url: form.ha_url.trim(),
        filabridge_public_url: form.filabridge_public_url.trim(),
        ...(form.ha_token.trim() ? { ha_token: form.ha_token.trim() } : {}),
      });
      toast.success("Configuração do Home Assistant salva");
      if (form.ha_token.trim()) setTokenSet(true);
      setForm((f) => ({ ...f, ha_token: "" }));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Falha ao salvar");
    } finally {
      setSaving(false);
    }
  };

  const test = async () => {
    setTesting(true);
    try {
      await api.testHA(form.ha_url.trim(), form.ha_token.trim());
      toast.success("Conexão com o Home Assistant OK");
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Falha na conexão com o HA"
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
          Conecte ao Home Assistant com a integração ha-bambulab para
          rastreamento automático de filamento
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <Field id="ha_url" label="URL do Home Assistant">
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
          label={`Token de acesso ${tokenSet ? "(já configurado)" : "(não definido)"}`}
          hint="Long-Lived Access Token — deixe em branco para manter o atual"
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
          label="URL pública do FilaBridge (webhooks e tags NFC/QR)"
          hint="Usada nos webhooks do HA e como base das URLs de tags NFC/QR — precisa ser acessível pela rede (não use localhost nem 0.0.0.0)"
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
            Salvar
          </Button>
          <Button variant="outline" onClick={test} disabled={testing}>
            {testing ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <PlugZap className="size-4" />
            )}
            Testar conexão
          </Button>
        </div>
        <p className="text-xs text-muted-foreground">
          Guia completo: <code>docs/home-assistant-setup.md</code> no
          repositório do FilaBridge.
        </p>
      </CardContent>
    </Card>
  );
}
