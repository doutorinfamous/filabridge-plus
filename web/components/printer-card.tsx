"use client";

import { ExternalLink, Printer } from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/api";
import type {
  PrinterConfigInfo,
  PrinterData,
  Spool,
  ToolheadMapping,
} from "@/lib/types";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { PrintJobSection, PrinterStateBadge } from "@/components/print-job";
import { SpoolSelect } from "@/components/spool-select";

interface PrinterCardProps {
  printerId: string;
  config: PrinterConfigInfo;
  data?: PrinterData;
  mappings?: Record<number, ToolheadMapping>;
  spools: Spool[];
  spoolmanUrl?: string;
  onChanged: () => void;
}

export function PrinterCard({
  config,
  data,
  mappings,
  spools,
  spoolmanUrl,
  onChanged,
}: PrinterCardProps) {
  const printerName = config.name;
  const toolheadIds = Array.from({ length: config.toolheads }, (_, i) => i);

  const findSpool = (spoolId?: number) =>
    spoolId ? spools.find((spool) => spool.id === spoolId) ?? null : null;

  const assign = async (toolheadId: number, spoolId: number) => {
    try {
      await api.mapToolhead(printerName, toolheadId, spoolId);
      toast.success(
        spoolId > 0
          ? `Spool ${spoolId} assigned to ${printerName}`
          : "Toolhead cleared"
      );
      onChanged();
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to assign spool"
      );
    }
  };

  return (
    <Card className="min-w-0 border-border/70 bg-card/60">
      <CardHeader className="flex flex-row items-start justify-between gap-3 space-y-0">
        <div className="flex min-w-0 flex-1 items-center gap-3">
          <div className="flex size-10 shrink-0 items-center justify-center rounded-xl border border-border/70 bg-background/60">
            <Printer className="size-5 text-muted-foreground" />
          </div>
          <div className="min-w-0 flex-1">
            <CardTitle className="truncate text-base">{data?.name ?? printerName}</CardTitle>
            <CardDescription>
              {config.model || "Snapmaker U1"} · Moonraker
              {config.toolheads > 1
                ? ` · ${config.toolheads} toolheads`
                : ""}
            </CardDescription>
          </div>
        </div>
        <PrinterStateBadge state={data?.state} className="shrink-0" />
      </CardHeader>
      <CardContent className="space-y-4">
        {data && <PrintJobSection data={data} />}

        <div className="space-y-2">
          <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
            Spools per toolhead
          </p>
          {toolheadIds.map((toolheadId) => {
            const mapping = mappings?.[toolheadId];
            const current = findSpool(mapping?.spool_id);
            const label =
              mapping?.display_name || `Toolhead ${toolheadId + 1}`;
            return (
              <div
                key={toolheadId}
                className="flex flex-col gap-1.5 sm:flex-row sm:items-center sm:gap-2"
              >
                <span className="truncate text-sm text-muted-foreground sm:w-28 sm:shrink-0">
                  {label}
                </span>
                <div className="min-w-0 w-full sm:flex-1">
                  <SpoolSelect
                    currentSpool={current}
                    loadAvailable={async () => {
                      const res = await api.getAvailableSpools({
                        printerName,
                        toolheadId,
                      });
                      return res.spools ?? [];
                    }}
                    onSelect={(spoolId) => assign(toolheadId, spoolId)}
                  />
                </div>
                {current && spoolmanUrl && (
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-9 shrink-0"
                    asChild
                  >
                    <a
                      href={`${spoolmanUrl}/spool/show/${current.id}`}
                      target="_blank"
                      rel="noreferrer"
                      title="Open in Spoolman"
                    >
                      <ExternalLink className="size-4" />
                    </a>
                  </Button>
                )}
              </div>
            );
          })}
        </div>
      </CardContent>
    </Card>
  );
}
