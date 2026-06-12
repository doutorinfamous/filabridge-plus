"use client";

import * as React from "react";
import {
  Check,
  Copy,
  ExternalLink,
  MapPin,
  Nfc,
  Palette,
  Search,
  Tag,
} from "lucide-react";
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
import { WriteNfcButton } from "@/components/nfc/write-nfc-button";
import { SpoolDot } from "@/components/spool-select";

type NfcTab = "spool" | "filament" | "location";

function normalizeHexColor(hex: string): string {
  const trimmed = hex.trim();
  if (!trimmed) return "";
  return trimmed.startsWith("#") ? trimmed : `#${trimmed}`;
}

function parseColorHex(hex?: string): string[] {
  if (!hex?.trim()) return ["#71717a"];

  const raw = hex.trim().startsWith("#") ? hex.trim().slice(1) : hex.trim();
  const parts = raw.split(/[,;|/]+/).map((part) => normalizeHexColor(part));
  const colors = parts.filter(Boolean);

  return colors.length > 0 ? colors : ["#71717a"];
}

function FilamentColorSwatch({
  hex,
  className,
}: {
  hex?: string;
  className?: string;
}) {
  const colors = parseColorHex(hex);

  if (colors.length === 1) {
    return <SpoolDot color={colors[0]} className={cn("size-4", className)} />;
  }

  const gradient =
    colors.length === 2
      ? `linear-gradient(135deg, ${colors[0]} 50%, ${colors[1]} 50%)`
      : `conic-gradient(${colors
          .map(
            (color, index) =>
              `${color} ${(index * 100) / colors.length}% ${((index + 1) * 100) / colors.length}%`
          )
          .join(", ")})`;

  return (
    <span
      className={cn(
        "inline-block size-4 shrink-0 rounded-full ring-1 ring-white/20",
        className
      )}
      style={{ background: gradient }}
    />
  );
}

function bambuPrinterName(entry: NfcUrlEntry): string {
  if (entry.printer_name?.trim()) return entry.printer_name.trim();
  const name = entry.display_name || entry.location_name || "";
  const idx = name.indexOf(" - ");
  return idx >= 0 ? name.slice(0, idx).trim() : "";
}

function bambuSlotLabel(entry: NfcUrlEntry): string {
  const name = entry.display_name || entry.location_name || "";
  const idx = name.indexOf(" - ");
  if (idx >= 0) return name.slice(idx + 3).trim();
  return name || "Slot AMS";
}

function entryTitle(entry: NfcUrlEntry): string {
  if (entry.type === "filament") {
    return `[${entry.filament_id}] ${entry.filament_name || "Unnamed filament"}`;
  }
  if (entry.type === "spool") {
    return `[${entry.spool_id}] ${entry.spool_name || "Unnamed spool"}`;
  }
  if (entry.location_type === "ams_slot") {
    return bambuSlotLabel(entry);
  }
  return entry.display_name || entry.location_name || "Location";
}

function entrySubtitle(entry: NfcUrlEntry): string {
  if (entry.type === "filament") {
    return `${entry.material || "?"} · ${entry.brand || "?"}`;
  }
  if (entry.type === "spool") {
    const weight =
      entry.remaining_weight != null
        ? ` · ${Math.round(entry.remaining_weight)}g`
        : "";
    return `${entry.material || "?"} · ${entry.brand || "?"}${weight}`;
  }
  if (entry.location_type === "ams_slot") {
    const printer = bambuPrinterName(entry);
    return printer ? `${printer} · Bambu AMS` : "Slot AMS (Bambu)";
  }
  return entry.location_type === "toolhead"
    ? "Printer toolhead"
    : "Storage location";
}

function entryKey(entry: NfcUrlEntry): string {
  return `${entry.type}-${entry.spool_id ?? entry.filament_id ?? entry.display_name ?? entry.location_name ?? entry.url}`;
}

export default function NfcPage() {
  const [entries, setEntries] = React.useState<NfcUrlEntry[] | null>(null);
  const [spoolmanUrl, setSpoolmanUrl] = React.useState("");
  const [tab, setTab] = React.useState<NfcTab>("spool");
  const [search, setSearch] = React.useState("");
  const [selected, setSelected] = React.useState<NfcUrlEntry | null>(null);
  const [copied, setCopied] = React.useState(false);
  const qrPanelRef = React.useRef<HTMLDivElement>(null);

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
          error instanceof Error ? error.message : "Failed to load NFC URLs"
        );
      });
  }, []);

  const filtered = React.useMemo(() => {
    return (entries ?? []).filter((entry) => {
      if (entry.type !== tab) return false;
      if (!search.trim()) return true;
      const haystack =
        `${entryTitle(entry)} ${entrySubtitle(entry)} ${entry.printer_name ?? ""} ${entry.display_name ?? ""}`.toLowerCase();
      return haystack.includes(search.trim().toLowerCase());
    });
  }, [entries, tab, search]);

  React.useEffect(() => {
    if (!selected || !qrPanelRef.current) return;
    if (window.matchMedia("(min-width: 1024px)").matches) return;

    qrPanelRef.current.scrollIntoView({ behavior: "smooth", block: "start" });
  }, [selected]);

  const copyUrl = async (url: string) => {
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
      toast.success("URL copied");
    } catch {
      toast.error("Could not copy");
    }
  };

  const switchTab = (next: string) => {
    setTab(next as NfcTab);
    setSelected(null);
    setSearch("");
  };

  const filamentUrl =
    selected &&
    selected.filament_id != null &&
    spoolmanUrl &&
    (selected.type === "spool" || selected.type === "filament")
      ? `${spoolmanUrl}/filament/show/${selected.filament_id}`
      : null;

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">NFC & QR</h1>
        <p className="text-sm text-muted-foreground">
          Generate URLs and QR codes to program NFC tags for spools, filaments,
          and locations
        </p>
      </header>

      <Tabs value={tab} onValueChange={switchTab}>
        <TabsList>
          <TabsTrigger value="spool">
            <Tag className="size-4" /> Spools
          </TabsTrigger>
          <TabsTrigger value="filament">
            <Palette className="size-4" /> Filaments
          </TabsTrigger>
          <TabsTrigger value="location">
            <MapPin className="size-4" /> Locations
          </TabsTrigger>
        </TabsList>
      </Tabs>

      <div className="grid min-w-0 gap-4 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)]">
        {/* List */}
        <Card className="min-w-0 border-border/70 bg-card/60 py-0">
          <CardContent className="p-3">
            <div className="relative mb-3">
              <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="Search..."
                className="pl-9"
              />
            </div>
            <ScrollArea className="h-52 w-full min-w-0 pr-2 sm:h-72 lg:h-[480px] [&>[data-slot=scroll-area-viewport]>div]:!block [&>[data-slot=scroll-area-viewport]>div]:min-w-0 [&>[data-slot=scroll-area-viewport]>div]:w-full">
              {entries === null ? (
                <div className="space-y-2">
                  {Array.from({ length: 6 }).map((_, i) => (
                    <Skeleton key={i} className="h-14 rounded-lg" />
                  ))}
                </div>
              ) : filtered.length === 0 ? (
                <p className="px-2 py-8 text-center text-sm text-muted-foreground">
                  Nothing found.
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
                          "flex w-full min-w-0 items-start gap-3 rounded-lg border px-3 py-2.5 text-left transition-colors",
                          isSelected
                            ? "border-primary/40 bg-accent"
                            : "border-transparent hover:bg-accent/50"
                        )}
                      >
                        <FilamentColorSwatch
                          hex={entry.color_hex}
                          className="mt-0.5"
                        />
                        <span className="min-w-0 flex-1">
                          <span className="line-clamp-2 text-sm font-medium leading-snug">
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
        <Card
          ref={qrPanelRef}
          className="min-w-0 overflow-hidden border-border/70 bg-card/60"
        >
          <CardContent className="flex w-full min-w-0 flex-col items-center justify-center gap-5 p-4 sm:p-6 lg:min-h-[520px]">
            {selected ? (
              <>
                <div className="flex w-full max-w-md min-w-0 items-start gap-3">
                  <FilamentColorSwatch
                    hex={selected.color_hex}
                    className="mt-1 shrink-0"
                  />
                  <div className="min-w-0 flex-1">
                    <h3 className="line-clamp-2 text-lg font-semibold leading-snug">
                      {entryTitle(selected)}
                    </h3>
                    <p className="truncate text-sm text-muted-foreground">
                      {entrySubtitle(selected)}
                    </p>
                  </div>
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
                    QR unavailable for this item.
                  </p>
                )}
                <div className="grid w-full max-w-md min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-2">
                  <code className="block min-w-0 truncate rounded-lg border border-border bg-background/60 px-3 py-2 text-xs">
                    {selected.url}
                  </code>
                  <Button
                    variant="outline"
                    size="icon"
                    onClick={() => copyUrl(selected.url)}
                    title="Copy URL"
                  >
                    {copied ? (
                      <Check className="size-4 text-success" />
                    ) : (
                      <Copy className="size-4" />
                    )}
                  </Button>
                </div>
                {/* key resets the write state when another item is selected */}
                <WriteNfcButton key={selected.url} url={selected.url} />
                <ol className="max-w-md list-decimal space-y-1 pl-5 text-xs text-muted-foreground">
                  <li>Open NFC Tools Pro on your phone</li>
                  <li>
                    Tap &quot;Write&quot; → &quot;Add a record&quot; → URL
                  </li>
                  <li>Scan this QR code (or paste the URL)</li>
                  <li>Write the NFC tag and test with your phone</li>
                </ol>
                {filamentUrl ? (
                  <a
                    href={filamentUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex items-center gap-1.5 text-xs text-primary hover:underline"
                  >
                    View filament in Spoolman
                    <ExternalLink className="size-3" />
                  </a>
                ) : null}
              </>
            ) : (
              <div className="text-center text-muted-foreground">
                <Nfc className="mx-auto mb-3 size-10 opacity-40" />
                <p className="text-sm font-medium">Select an item</p>
                <p className="text-xs">
                  Choose an item from the list to generate the QR code
                </p>
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
