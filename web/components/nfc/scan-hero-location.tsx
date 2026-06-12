"use client";

import { Box, Check, Layers, MapPin, Printer } from "lucide-react";

import type { NfcSessionLocation } from "@/lib/types";
import { cn } from "@/lib/utils";

function locationIcon(type: NfcSessionLocation["location_type"]) {
  switch (type) {
    case "toolhead":
      return Printer;
    case "ams_slot":
      return Layers;
    default:
      return Box;
  }
}

function locationLabel(type: NfcSessionLocation["location_type"]) {
  switch (type) {
    case "toolhead":
      return "toolhead";
    case "ams_slot":
      return "AMS slot";
    default:
      return "storage";
  }
}

export function ScanHeroLocation({
  location,
  confirmed = true,
  className,
}: {
  location: NfcSessionLocation;
  confirmed?: boolean;
  className?: string;
}) {
  const Icon = locationIcon(location.location_type);
  const subtitle =
    location.location_type === "toolhead"
      ? [location.printer_name, location.toolhead_display_name]
          .filter(Boolean)
          .join(" · ")
      : location.location_type === "ams_slot"
        ? location.printer_name ?? "Bambu AMS"
        : "Spoolman location";

  return (
    <div
      className={cn(
        "animate-in fade-in slide-in-from-bottom-4 duration-500 rounded-2xl border border-cyan-400/20 bg-white/[0.04] p-6 backdrop-blur-xl",
        className
      )}
      style={{
        boxShadow:
          "0 0 40px color-mix(in oklch, oklch(0.65 0.18 220) 30%, transparent)",
      }}
    >
      <div className="mb-4 flex items-center justify-between">
        <span className="font-mono text-[10px] uppercase tracking-[0.2em] text-cyan-300/70">
          location identified
        </span>
        {confirmed && (
          <span className="flex size-6 items-center justify-center rounded-full bg-emerald-500/20 text-emerald-400">
            <Check className="size-3.5" strokeWidth={3} />
          </span>
        )}
      </div>

      <div className="relative mx-auto flex size-36 items-center justify-center">
        <div className="absolute inset-0 rounded-full bg-cyan-400/20 blur-2xl" />
        <div className="relative flex size-28 items-center justify-center rounded-2xl border border-cyan-400/30 bg-gradient-to-br from-cyan-500/20 to-violet-500/10">
          <Icon className="size-14 text-cyan-300/90" strokeWidth={1.25} />
          <MapPin className="absolute -bottom-1 -right-1 size-6 text-violet-300/80" />
        </div>
      </div>

      <div className="mt-5 space-y-1 text-center">
        <p className="text-lg font-semibold tracking-tight">
          {location.display_name || location.name}
        </p>
        <p className="text-sm text-white/60">{subtitle}</p>
        <p className="font-mono text-xs uppercase tracking-wider text-cyan-300/50">
          {locationLabel(location.location_type)}
        </p>
      </div>
    </div>
  );
}
