"use client";

import type { CSSProperties } from "react";
import { Check } from "lucide-react";

import type { NfcSessionSpool } from "@/lib/types";
import { cn } from "@/lib/utils";

function normalizeColor(hex?: string): string {
  if (!hex) return "#71717a";
  return hex.startsWith("#") ? hex : `#${hex}`;
}

function SpoolVisual({ color }: { color: string }) {
  return (
    <div className="relative mx-auto size-36">
      <div
        className="absolute inset-0 rounded-full blur-2xl"
        style={{ backgroundColor: color, opacity: 0.45 }}
      />
      <svg
        viewBox="0 0 120 120"
        className="relative size-full drop-shadow-[0_0_24px_color-mix(in_srgb,var(--spool-color)_50%,transparent)]"
        style={{ "--spool-color": color } as CSSProperties}
        aria-hidden
      >
        <defs>
          <linearGradient id="spool-rim" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="oklch(0.85 0 0)" />
            <stop offset="100%" stopColor="oklch(0.55 0 0)" />
          </linearGradient>
          <linearGradient id="spool-fill" x1="0%" y1="0%" x2="0%" y2="100%">
            <stop offset="0%" stopColor={color} stopOpacity="0.95" />
            <stop offset="100%" stopColor={color} stopOpacity="0.55" />
          </linearGradient>
        </defs>
        <ellipse cx="60" cy="60" rx="48" ry="48" fill="url(#spool-rim)" opacity="0.9" />
        <ellipse cx="60" cy="60" rx="38" ry="38" fill="url(#spool-fill)" />
        <ellipse cx="60" cy="60" rx="14" ry="14" fill="oklch(0.15 0.01 270)" />
        <ellipse cx="60" cy="60" rx="10" ry="10" fill="oklch(0.25 0.01 270)" />
        <path
          d="M 60 22 A 38 38 0 0 1 60 98"
          fill="none"
          stroke="oklch(1 0 0 / 0.15)"
          strokeWidth="3"
        />
      </svg>
    </div>
  );
}

export function ScanHeroSpool({
  spool,
  confirmed = true,
  className,
}: {
  spool: NfcSessionSpool;
  confirmed?: boolean;
  className?: string;
}) {
  const color = normalizeColor(spool.color_hex);
  const weight =
    spool.remaining_weight != null
      ? `${Math.round(spool.remaining_weight)}g`
      : null;

  return (
    <div
      className={cn(
        "animate-in fade-in slide-in-from-bottom-4 duration-500 rounded-2xl border border-white/10 bg-white/[0.04] p-6 backdrop-blur-xl",
        className
      )}
      style={{ boxShadow: `0 0 40px color-mix(in srgb, ${color} 25%, transparent)` }}
    >
      <div className="mb-4 flex items-center justify-between">
        <span className="font-mono text-[10px] uppercase tracking-[0.2em] text-white/50">
          spool identified
        </span>
        {confirmed && (
          <span className="flex size-6 items-center justify-center rounded-full bg-emerald-500/20 text-emerald-400">
            <Check className="size-3.5" strokeWidth={3} />
          </span>
        )}
      </div>

      <SpoolVisual color={color} />

      <div className="mt-5 space-y-1 text-center">
        <p className="text-lg font-semibold tracking-tight">
          {spool.name?.trim() || `Spool #${spool.id}`}
        </p>
        <p className="text-sm text-white/60">
          {[spool.material, spool.brand].filter(Boolean).join(" · ") || "Filament"}
          {weight ? ` · ${weight}` : ""}
        </p>
        <p className="font-mono text-xs text-white/40">ID {spool.id}</p>
      </div>
    </div>
  );
}
