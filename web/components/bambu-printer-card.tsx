"use client";

import { Boxes, ExternalLink } from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/api";
import { findSpoolForBambuTray } from "@/lib/bambu-tray-spool";
import type { BambuPrinter, BambuTray, Spool } from "@/lib/types";
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

interface BambuPrinterCardProps {
  printer: BambuPrinter;
  spools: Spool[];
  spoolmanUrl?: string;
  onChanged: () => void;
}

function trayLabel(tray: BambuTray, amsName?: string): string {
  if (tray.display_name) {
    // "Printer - AMS 1 Slot 2" → keep only the part after the printer name
    const idx = tray.display_name.indexOf(" - ");
    return idx >= 0 ? tray.display_name.slice(idx + 3) : tray.display_name;
  }
  if (tray.is_external) return "External Spool";
  return `${amsName ?? `AMS ${tray.ams_number}`} Slot ${tray.tray_number}`;
}

export function BambuPrinterCard({
  printer,
  spools,
  spoolmanUrl,
  onChanged,
}: BambuPrinterCardProps) {
  const assign = async (tray: BambuTray, spoolId: number) => {
    try {
      await api.assignTray(tray.unique_id, spoolId);
      toast.success(
        spoolId > 0
          ? `Spool ${spoolId} assigned to ${trayLabel(tray)}`
          : "Tray cleared"
      );
      onChanged();
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to assign spool"
      );
    }
  };

  const renderTrayRow = (tray: BambuTray, amsName?: string) => {
    const current = findSpoolForBambuTray(tray, spools);
    return (
      <div
        key={tray.unique_id}
        className="flex flex-col gap-1.5 sm:flex-row sm:items-center sm:gap-2"
      >
        <span className="truncate text-sm text-muted-foreground sm:w-28 sm:shrink-0">
          {trayLabel(tray, amsName)}
        </span>
        <div className="min-w-0 w-full sm:flex-1">
          <SpoolSelect
            currentSpool={current}
            loadAvailable={async () => {
              const res = await api.getAvailableSpools({
                trayUniqueId: tray.unique_id,
              });
              return res.spools ?? [];
            }}
            onSelect={(spoolId) => assign(tray, spoolId)}
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
  };

  const amsUnits = printer.ams_units ?? [];
  const externalSpools = printer.external_spools ?? [];
  const hasTrays = amsUnits.length > 0 || externalSpools.length > 0;

  return (
    <Card className="min-w-0 border-border/70 bg-card/60">
      <CardHeader className="flex flex-row items-start justify-between gap-3 space-y-0">
        <div className="flex min-w-0 flex-1 items-center gap-3">
          <div className="flex size-10 shrink-0 items-center justify-center rounded-xl border border-emerald-500/30 bg-emerald-500/10">
            <Boxes className="size-5 text-emerald-400" />
          </div>
          <div className="min-w-0 flex-1">
            <CardTitle className="truncate text-base">{printer.name}</CardTitle>
            <CardDescription>Bambu Lab · Home Assistant</CardDescription>
          </div>
        </div>
        <PrinterStateBadge state={printer.state} className="shrink-0" />
      </CardHeader>
      <CardContent className="space-y-4">
        <PrintJobSection
          data={{
            name: printer.name,
            state: printer.state ?? "IDLE",
            job_name: printer.job_name,
            progress: printer.progress,
            print_duration: printer.print_duration,
            time_remaining: printer.time_remaining,
            current_layer: printer.current_layer,
            total_layer: printer.total_layer,
          }}
        />

        {hasTrays ? (
          <div className="space-y-4">
            {amsUnits.map((ams) => (
              <div key={`${ams.ams_number}-${ams.entity_id}`} className="space-y-2">
                <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  {ams.name}
                </p>
                {ams.trays.map((tray) => renderTrayRow(tray, ams.name))}
              </div>
            ))}
            {externalSpools.length > 0 && (
              <div className="space-y-2">
                <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  External Spool
                </p>
                {externalSpools.map((tray) => renderTrayRow(tray))}
              </div>
            )}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">
            No trays discovered yet.
          </p>
        )}
      </CardContent>
    </Card>
  );
}
