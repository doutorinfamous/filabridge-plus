"use client";

import * as React from "react";
import { Loader2 } from "lucide-react";

import { ScanConnectionBeam } from "@/components/nfc/scan-connection-beam";
import { ScanHeroLocation } from "@/components/nfc/scan-hero-location";
import { ScanWaitingCard } from "@/components/nfc/scan-waiting-card";
import type {
  NfcSessionFilament,
  NfcSessionLocation,
  NfcSessionSpool,
} from "@/lib/types";
import { cn } from "@/lib/utils";

function normalizeColor(hex?: string): string {
  if (!hex) return "#71717a";
  return hex.startsWith("#") ? hex : `#${hex}`;
}

function SpoolOption({
  spool,
  selected,
  disabled,
  onSelect,
}: {
  spool: NfcSessionSpool;
  selected: boolean;
  disabled: boolean;
  onSelect: () => void;
}) {
  const color = normalizeColor(spool.color_hex);
  const weight =
    spool.remaining_weight != null
      ? `${Math.round(spool.remaining_weight)}g`
      : null;

  return (
    <button
      type="button"
      onClick={onSelect}
      disabled={disabled}
      className={cn(
        "flex w-full items-center gap-3 rounded-xl border px-4 py-3 text-left transition-colors",
        selected
          ? "border-emerald-400/40 bg-emerald-500/10"
          : "border-white/10 bg-white/[0.03] hover:border-white/20 hover:bg-white/[0.06]",
        disabled && "cursor-not-allowed opacity-60"
      )}
    >
      <span
        className="size-4 shrink-0 rounded-full ring-1 ring-white/20"
        style={{ backgroundColor: color }}
      />
      <span className="min-w-0 flex-1">
        <span className="block font-mono text-sm font-semibold">#{spool.id}</span>
        <span className="block truncate text-xs text-white/60">
          {[weight, spool.location].filter(Boolean).join(" · ") ||
            "No location"}
        </span>
      </span>
    </button>
  );
}

export function ScanSpoolPicker({
  filament,
  location,
  selecting,
  onSelect,
}: {
  filament: NfcSessionFilament;
  location?: NfcSessionLocation;
  selecting: boolean;
  onSelect: (spoolId: number) => void;
}) {
  const [selectedId, setSelectedId] = React.useState<number | null>(null);

  const handleSelect = (spoolId: number) => {
    if (selecting) return;
    setSelectedId(spoolId);
    onSelect(spoolId);
  };

  return (
    <div className="flex flex-1 flex-col px-5 py-8">
      <header className="mb-8 text-center">
        <p className="font-mono text-[10px] uppercase tracking-[0.3em] text-white/40">
          FilaBridge+ · NFC
        </p>
        <h1 className="mt-2 text-xl font-semibold tracking-tight">
          Choose spool
        </h1>
        <p className="mt-1 text-sm text-white/50">
          {filament.name?.trim() || `Filament #${filament.id}`} has{" "}
          {filament.candidates.length} available spools
        </p>
      </header>

      <div className="mx-auto w-full max-w-md flex-1 space-y-4">
        {location ? (
          <>
            <ScanHeroLocation location={location} />
            <ScanConnectionBeam />
          </>
        ) : null}

        <div className="rounded-2xl border border-white/10 bg-white/[0.04] p-4 backdrop-blur-xl">
          <p className="mb-3 font-mono text-[10px] uppercase tracking-[0.2em] text-white/50">
            available spools
          </p>
          <div className="space-y-2">
            {filament.candidates.map((spool) => (
              <SpoolOption
                key={spool.id}
                spool={spool}
                selected={selectedId === spool.id}
                disabled={selecting}
                onSelect={() => handleSelect(spool.id)}
              />
            ))}
          </div>
          {selecting ? (
            <div className="mt-4 flex items-center justify-center gap-2 text-sm text-white/60">
              <Loader2 className="size-4 animate-spin" />
              Confirming spool...
            </div>
          ) : null}
        </div>

        {!location ? <ScanWaitingCard kind="location" /> : null}
      </div>
    </div>
  );
}
