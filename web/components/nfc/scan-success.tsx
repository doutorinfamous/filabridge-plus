"use client";

import Link from "next/link";
import { ArrowRight, CheckCircle2 } from "lucide-react";

import { SpoolDot } from "@/components/spool-select";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export interface ScanSuccessData {
  spoolId: number;
  spoolName?: string;
  spoolMaterial?: string;
  spoolBrand?: string;
  spoolColor?: string;
  spoolWeight?: number;
  location: string;
  locationType?: "storage" | "ams_slot" | "toolhead";
  printer?: string;
  toolhead?: string;
  slot?: string;
}

function normalizeColor(hex?: string): string {
  if (!hex) return "#71717a";
  return hex.startsWith("#") ? hex : `#${hex}`;
}

function locationSummary(data: ScanSuccessData): string {
  if (data.locationType === "ams_slot") {
    return [data.printer, data.slot ?? data.location].filter(Boolean).join(" · ");
  }
  if (data.locationType === "toolhead") {
    return [data.printer, data.toolhead ?? data.location].filter(Boolean).join(" · ");
  }
  return data.location;
}

function spoolSubtitle(data: ScanSuccessData): string {
  const parts = [data.spoolMaterial, data.spoolBrand].filter(Boolean);
  if (data.spoolWeight != null) {
    parts.push(`${Math.round(data.spoolWeight)}g`);
  }
  return parts.join(" · ");
}

export function ScanSuccess({
  data,
  className,
}: {
  data: ScanSuccessData;
  className?: string;
}) {
  const color = normalizeColor(data.spoolColor);
  const subtitle = spoolSubtitle(data);
  const spoolTitle = data.spoolName?.trim() || `Spool #${data.spoolId}`;

  return (
    <div
      className={cn(
        "animate-in zoom-in-95 fade-in duration-500 flex flex-1 flex-col items-center justify-center px-6 py-10 text-center",
        className
      )}
    >
      <div className="relative mb-8">
        <div className="absolute inset-0 scale-150 rounded-full bg-emerald-500/20 blur-3xl" />
        <CheckCircle2
          className="relative size-20 text-emerald-400"
          strokeWidth={1.25}
        />
      </div>

      <h1 className="text-2xl font-semibold tracking-tight">Assignment complete</h1>
      <p className="mt-2 max-w-xs text-sm text-white/60">
        The spool has been successfully linked to the location.
      </p>

      <div className="mt-8 w-full max-w-sm space-y-3 rounded-2xl border border-white/10 bg-white/[0.04] p-5 text-left backdrop-blur-xl">
        <div className="space-y-2">
          <span className="text-xs font-medium uppercase tracking-wider text-white/40">
            Spool
          </span>
          <div className="flex items-start gap-3">
            <SpoolDot color={color} className="mt-1 size-4 shrink-0" />
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
                <span className="font-mono text-sm font-semibold">#{data.spoolId}</span>
                <span className="text-sm font-medium leading-snug">{spoolTitle}</span>
              </div>
              {subtitle ? (
                <p className="mt-1 text-sm text-white/55">{subtitle}</p>
              ) : null}
            </div>
          </div>
        </div>

        <div className="flex items-center justify-center py-1">
          <ArrowRight className="size-4 text-emerald-400/70" />
        </div>

        <div className="space-y-1">
          <span className="text-xs font-medium uppercase tracking-wider text-white/40">
            Location
          </span>
          <p className="text-sm font-medium leading-snug">{locationSummary(data)}</p>
        </div>
      </div>

      <Button asChild className="mt-10 min-h-11 w-full max-w-sm" size="lg">
        <Link href="/">Back to dashboard</Link>
      </Button>
    </div>
  );
}
