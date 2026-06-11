"use client";

import * as React from "react";
import { Check, Copy, ExternalLink, MapPin, Nfc, Search, Tag } from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/api";
import type { NfcUrlEntry } from "@/lib/types";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";

type NfcTab = "spool" | "location";

function entryTitle(entry: NfcUrlEntry): string {
  if (entry.type === "spool") {
    return `[${entry.spool_id}] ${entry.spool_name || "Spool sem nome"}`;
  }
  return entry.display_name || entry.location_name || "Local";
}

function entrySubtitle(entry: NfcUrlEntry): string {
  if (entry.type === "spool") {
    const weight =
      entry.remaining_weight != null
        ? ` · ${Math.round(entry.remaining_weight)}g`
        : "";
    return `${entry.material || "?"} · ${entry.brand || "?"}${weight}`;
  }
  return entry.location_type === "ams_slot"
    ? "Slot AMS (Bambu)"
    : "Local de armazenamento";
}

function entryKey(entry: NfcUrlEntry): string {
  return `${entry.type}-${entry.spool_id ?? entry.display_name ?? entry.location_name ?? entry.url}`;
}

function ColorDot({ hex }: { hex?: string }) {
  const color = hex ? (hex.startsWith("#") ? hex : `#${hex}`) : "#52525b";
  return (
    <span
      className="size-3 shrink-0 rounded-full ring-1 ring-white/20"
      style={{ backgroundColor: color }}
    />
  );
}

export default function NfcPage() {
  const [entries, setEntries] = React.useState<NfcUrlEntry[] | null>(null);
  const [spoolmanUrl, setSpoolmanUrl] = React.useState("");
  const [tab, setTab] = React.useState<NfcTab>("spool");
  const [search, setSearch] = React.useState("");
  const [selected, setSelected] = React.useState<NfcUrlEntry | null>(null);
  const [copied, setCopied] = React.useState(false);

  React.useEffect(() => {
    api
      .getNfcUrls()
      .then((res) => {
        setEntries(res.urls ?? []);
        setSpoolmanUrl((res.spoolman_url ?? "").replace(/\/$/, ""));
      })
      .catch((error) => {
        setEntries([]);
        toast.error(
          error instanceof Error ? error.message : "Falha ao carregar URLs NFC"
        );
      });
  }, []);

  const filtered = React.useMemo(() => {
    return (entries ?? []).filter((entry) => {
      if (entry.type !== tab) return false;
      if (!search.trim()) return true;
      const haystack =
        `${entryTitle(entry)} ${entrySubtitle(entry)}`.toLowerCase();
      return haystack.includes(search.trim().toLowerCase());
    });
  }, [entries, tab, search]);

  const copyUrl = async (url: string) => {
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
      toast.success("URL copiada");
    } catch {
      toast.error("Não foi possível copiar");
    }
  };

  const switchTab = (next: string) => {
    setTab(next as NfcTab);
    setSelected(null);
    setSearch("");
  };

  const filamentUrl =
    selected?.type === "spool" &&
    selected.filament_id != null &&
    spoolmanUrl
      ? `${spoolmanUrl}/filament/show/${selected.filament_id}`
      : null;

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">NFC & QR</h1>
        <p className="text-sm text-muted-foreground">
          Gere URLs e QR codes para programar tags NFC de spools e locais
        </p>
      </header>

      <Tabs value={tab} onValueChange={switchTab}>
        <TabsList>
          <TabsTrigger value="spool">
            <Tag className="size-4" /> Spools
          </TabsTrigger>
          <TabsTrigger value="location">
            <MapPin className="size-4" /> Locais
          </TabsTrigger>
        </TabsList>
      </Tabs>

      <div className="grid gap-4 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)]">
        {/* List */}
        <Card className="border-border/70 bg-card/60 py-0">
          <CardContent className="p-3">
            <div className="relative mb-3">
              <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="Buscar..."
                className="pl-9"
              />
            </div>
            <ScrollArea className="h-[480px] pr-2">
              {entries === null ? (
                <div className="space-y-2">
                  {Array.from({ length: 6 }).map((_, i) => (
                    <Skeleton key={i} className="h-14 rounded-lg" />
                  ))}
                </div>
              ) : filtered.length === 0 ? (
                <p className="px-2 py-8 text-center text-sm text-muted-foreground">
                  Nada encontrado.
                </p>
              ) : (
                <div key={tab} className="space-y-1">
                  {filtered.map((entry) => {
                    const isSelected = selected?.url === entry.url;
                    return (
                      <button
                        type="button"
                        key={entryKey(entry)}
                        onClick={() => setSelected(entry)}
                        className={cn(
                          "flex w-full items-center gap-3 rounded-lg border px-3 py-2.5 text-left transition-colors",
                          isSelected
                            ? "border-primary/40 bg-accent"
                            : "border-transparent hover:bg-accent/50"
                        )}
                      >
                        <ColorDot hex={entry.color_hex} />
                        <span className="min-w-0">
                          <span className="block truncate text-sm font-medium">
                            {entryTitle(entry)}
                          </span>
                          <span className="block truncate text-xs text-muted-foreground">
                            {entrySubtitle(entry)}
                          </span>
                        </span>
                      </button>
                    );
                  })}
                </div>
              )}
            </ScrollArea>
          </CardContent>
        </Card>

        {/* QR display */}
        <Card className="border-border/70 bg-card/60">
          <CardContent className="flex min-h-[520px] flex-col items-center justify-center gap-5 p-6">
            {selected ? (
              <>
                <div className="text-center">
                  <h3 className="text-lg font-semibold">
                    {entryTitle(selected)}
                  </h3>
                  <p className="text-sm text-muted-foreground">
                    {entrySubtitle(selected)}
                  </p>
                </div>
                {selected.qr_code_base64 ? (
                  <div className="rounded-2xl bg-white p-4 shadow-lg">
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    <img
                      src={`data:image/png;base64,${selected.qr_code_base64}`}
                      alt="QR Code"
                      width={256}
                      height={256}
                      className="size-56"
                    />
                  </div>
                ) : (
                  <p className="text-sm text-muted-foreground">
                    QR indisponível para este item.
                  </p>
                )}
                <div className="flex w-full max-w-md items-center gap-2">
                  <code className="min-w-0 flex-1 truncate rounded-lg border border-border bg-background/60 px-3 py-2 text-xs">
                    {selected.url}
                  </code>
                  <Button
                    variant="outline"
                    size="icon"
                    onClick={() => copyUrl(selected.url)}
                    title="Copiar URL"
                  >
                    {copied ? (
                      <Check className="size-4 text-success" />
                    ) : (
                      <Copy className="size-4" />
                    )}
                  </Button>
                </div>
                <ol className="max-w-md list-decimal space-y-1 pl-5 text-xs text-muted-foreground">
                  <li>Abra o NFC Tools Pro no celular</li>
                  <li>
                    Toque em &quot;Escrever&quot; → &quot;Adicionar registro&quot; →
                    URL
                  </li>
                  <li>Escaneie este QR code (ou cole a URL)</li>
                  <li>Grave a tag NFC e teste com o celular</li>
                </ol>
                {filamentUrl ? (
                  <a
                    href={filamentUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex items-center gap-1.5 text-xs text-primary hover:underline"
                  >
                    Ver filamento no Spoolman
                    <ExternalLink className="size-3" />
                  </a>
                ) : null}
              </>
            ) : (
              <div className="text-center text-muted-foreground">
                <Nfc className="mx-auto mb-3 size-10 opacity-40" />
                <p className="text-sm font-medium">Selecione um item</p>
                <p className="text-xs">
                  Escolha um item na lista para gerar o QR code
                </p>
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
