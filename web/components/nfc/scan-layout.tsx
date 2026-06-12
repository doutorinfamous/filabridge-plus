"use client";

import * as React from "react";

import { cn } from "@/lib/utils";

export function ScanLayout({
  children,
  accentColor,
  className,
}: {
  children: React.ReactNode;
  accentColor?: string;
  className?: string;
}) {
  const glow = accentColor ?? "oklch(0.65 0.18 220)";

  return (
    <div
      className={cn(
        "relative flex min-h-dvh flex-col overflow-hidden bg-[oklch(0.11_0.02_270)] text-foreground",
        className
      )}
      style={
        {
          "--scan-accent": glow,
          paddingTop: "env(safe-area-inset-top)",
          paddingBottom: "env(safe-area-inset-bottom)",
          paddingLeft: "env(safe-area-inset-left)",
          paddingRight: "env(safe-area-inset-right)",
        } as React.CSSProperties
      }
    >
      <div
        className="pointer-events-none absolute inset-0 opacity-40"
        style={{
          background: `
            radial-gradient(ellipse 80% 50% at 50% -10%, color-mix(in oklch, var(--scan-accent) 35%, transparent), transparent),
            radial-gradient(ellipse 60% 40% at 100% 80%, oklch(0.45 0.2 300 / 0.25), transparent),
            radial-gradient(ellipse 50% 30% at 0% 60%, oklch(0.55 0.15 200 / 0.2), transparent)
          `,
        }}
      />
      <div
        className="pointer-events-none absolute inset-0 opacity-[0.07]"
        style={{
          backgroundImage: `
            linear-gradient(oklch(1 0 0 / 0.5) 1px, transparent 1px),
            linear-gradient(90deg, oklch(1 0 0 / 0.5) 1px, transparent 1px)
          `,
          backgroundSize: "32px 32px",
        }}
      />
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(ellipse_at_center,transparent_0%,oklch(0.08_0.02_270)_75%)]" />

      <div className="relative z-10 flex min-h-dvh flex-1 flex-col">{children}</div>
    </div>
  );
}
