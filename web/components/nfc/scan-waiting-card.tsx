"use client";

import { Box, CircleDot, Layers, Loader2, Printer } from "lucide-react";

import { cn } from "@/lib/utils";

type WaitingKind = "spool" | "location";

function WaitingIcon({ kind }: { kind: WaitingKind }) {
  const Icon =
    kind === "location"
      ? Box
      : CircleDot;
  const Secondary =
    kind === "location" ? Layers : Printer;

  return (
    <div className="relative flex size-16 items-center justify-center rounded-xl border border-dashed border-white/15 bg-white/[0.02]">
      <Icon className="size-8 text-white/20" strokeWidth={1.25} />
      <Secondary className="absolute -bottom-1 -right-1 size-4 text-white/15" />
    </div>
  );
}

export function ScanWaitingCard({
  kind,
  className,
}: {
  kind: WaitingKind;
  className?: string;
}) {
  const title =
    kind === "location" ? "Waiting for location" : "Waiting for spool";
  const monoLabel =
    kind === "location" ? "loading location" : "loading spool";

  return (
    <div
      className={cn(
        "animate-in fade-in slide-in-from-bottom-6 duration-700 delay-150 rounded-2xl border border-dashed border-white/10 bg-white/[0.02] p-6 backdrop-blur-sm",
        className
      )}
    >
      <div className="mb-4 flex items-center gap-2">
        <Loader2 className="size-3.5 animate-spin text-white/40" />
        <span className="font-mono text-[10px] uppercase tracking-[0.25em] text-white/35">
          {monoLabel}
        </span>
      </div>

      <div className="flex items-center gap-4">
        <WaitingIcon kind={kind} />
        <div className="min-w-0 flex-1 space-y-2">
          <p className="text-sm font-medium text-white/70">{title}</p>
          <p className="text-xs text-white/40">
            {kind === "location"
              ? "Scan the destination NFC tag or QR code"
              : "Scan the spool NFC tag or QR code"}
          </p>
          <div className="flex gap-1 pt-1">
            {[0, 1, 2].map((i) => (
              <span
                key={i}
                className="size-1.5 rounded-full bg-white/25 animate-pulse"
                style={{ animationDelay: `${i * 200}ms` }}
              />
            ))}
          </div>
        </div>
      </div>

      <div className="mt-4 h-1 overflow-hidden rounded-full bg-white/5">
        <div className="h-full w-1/3 animate-[shimmer_2s_ease-in-out_infinite] rounded-full bg-gradient-to-r from-transparent via-white/30 to-transparent" />
      </div>
    </div>
  );
}
