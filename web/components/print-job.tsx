"use client";

import { Clock, Layers, Timer } from "lucide-react";

import { formatDuration, progressPercent } from "@/lib/api";
import type { PrinterData } from "@/lib/types";
import { Badge } from "@/components/ui/badge";
import { Progress } from "@/components/ui/progress";
import { cn } from "@/lib/utils";

export function PrinterStateBadge({
  state,
  className,
}: {
  state?: string;
  className?: string;
}) {
  const normalized = (state ?? "idle").toLowerCase();
  const styles: Record<string, string> = {
    printing: "bg-success/15 text-success border-success/30",
    idle: "bg-muted text-muted-foreground border-border",
    finished: "bg-sky-500/15 text-sky-400 border-sky-500/30",
    offline: "bg-destructive/15 text-destructive border-destructive/30",
    error: "bg-destructive/15 text-destructive border-destructive/30",
    not_configured: "bg-warning/15 text-warning border-warning/30",
  };
  return (
    <Badge
      variant="outline"
      className={cn(
        "uppercase tracking-wide",
        styles[normalized] ?? styles.idle,
        className
      )}
    >
      <span
        className={cn(
          "mr-0.5 size-1.5 rounded-full",
          normalized === "printing" && "animate-pulse bg-success",
          (normalized === "offline" || normalized === "error") &&
            "bg-destructive",
          normalized === "finished" && "bg-sky-400",
          !["printing", "offline", "error", "finished"].includes(normalized) &&
            "bg-muted-foreground"
        )}
      />
      {state ?? "IDLE"}
    </Badge>
  );
}

export function PrintJobSection({ data }: { data: PrinterData }) {
  if ((data.state ?? "").toUpperCase() !== "PRINTING") return null;

  const pct = progressPercent(data.progress);

  return (
    <div className="min-w-0 space-y-2 overflow-hidden rounded-lg border border-border/60 bg-background/40 p-3">
      <div className="flex min-w-0 items-center justify-between gap-2">
        <p className="min-w-0 flex-1 truncate text-sm font-medium">
          {data.job_name || "Job in progress"}
        </p>
        <span className="shrink-0 text-sm font-semibold tabular-nums">
          {pct.toFixed(1)}%
        </span>
      </div>
      <div className="min-w-0">
        <Progress value={pct} className="h-1.5" />
      </div>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
        <span className="flex items-center gap-1">
          <Clock className="size-3" />
          Elapsed: {formatDuration(data.print_duration)}
        </span>
        <span className="flex items-center gap-1">
          <Timer className="size-3" />
          Remaining: {formatDuration(data.time_remaining)}
        </span>
        {data.current_layer != null && data.total_layer != null && (
          <span className="flex items-center gap-1">
            <Layers className="size-3" />
            Layer {data.current_layer} / {data.total_layer}
          </span>
        )}
      </div>
    </div>
  );
}
