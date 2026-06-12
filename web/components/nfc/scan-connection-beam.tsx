"use client";

import { cn } from "@/lib/utils";

export function ScanConnectionBeam({ className }: { className?: string }) {
  return (
    <div className={cn("flex flex-col items-center py-3", className)}>
      <div className="relative h-14 w-px overflow-hidden bg-white/10">
        <div
          className="absolute inset-x-0 h-8 animate-[scan-beam_1.8s_ease-in-out_infinite]"
          style={{
            background:
              "linear-gradient(to bottom, transparent, var(--scan-accent, oklch(0.65 0.18 220)), transparent)",
            boxShadow: "0 0 12px color-mix(in oklch, var(--scan-accent) 60%, transparent)",
          }}
        />
      </div>
      <span className="mt-2 font-mono text-[10px] uppercase tracking-[0.25em] text-white/40">
        pairing
      </span>
    </div>
  );
}
