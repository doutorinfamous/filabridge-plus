"use client";

import * as React from "react";
import Link from "next/link";
import {
  CloudOff,
  Database,
  PlugZap,
  Printer,
  Settings,
  Wifi,
  WifiOff,
} from "lucide-react";

import { api } from "@/lib/api";
import type { BambuPrinter, PrinterConfigInfo } from "@/lib/types";
import { usePollInterval } from "@/lib/use-poll-interval";
import { useStatusSocket } from "@/lib/use-status-socket";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { BambuPrinterCard } from "@/components/bambu-printer-card";
import { PrintErrors } from "@/components/print-errors";
import { PrinterCard } from "@/components/printer-card";

export default function DashboardPage() {
  const { intervalMs } = usePollInterval();
  const { status, connected, refresh } = useStatusSocket(intervalMs);

  const [printers, setPrinters] = React.useState<Record<
    string,
    PrinterConfigInfo
  > | null>(null);
  const [bambuPrinters, setBambuPrinters] = React.useState<BambuPrinter[]>([]);
  const [bambuError, setBambuError] = React.useState<string | null>(null);
  const [spoolmanOk, setSpoolmanOk] = React.useState<boolean | null>(null);
  const [spoolmanUrl, setSpoolmanUrl] = React.useState<string>("");
  const [spoolmanConfigured, setSpoolmanConfigured] = React.useState<
    boolean | null
  >(null);

  const loadPrinters = React.useCallback(async () => {
    try {
      const res = await api.getPrinters();
      setPrinters(res.printers ?? {});
    } catch {
      setPrinters({});
    }
  }, []);

  const loadBambu = React.useCallback(async () => {
    try {
      const list = await api.getBambuPrinters();
      setBambuPrinters((list ?? []).filter((p) => p.registered));
      setBambuError(null);
    } catch (error) {
      setBambuPrinters([]);
      setBambuError(error instanceof Error ? error.message : "error");
    }
  }, []);

  React.useEffect(() => {
    const initialTimer = setTimeout(() => {
      loadPrinters();
      loadBambu();
      api
        .testSpoolman()
        .then(() => setSpoolmanOk(true))
        .catch(() => setSpoolmanOk(false));
      api
        .getConfig()
        .then((cfg) => {
          const url = (cfg.spoolman_url ?? "").replace(/\/$/, "");
          setSpoolmanUrl(url);
          setSpoolmanConfigured(url !== "");
        })
        .catch(() => undefined);
    }, 0);
    const timer = setInterval(loadBambu, intervalMs);

    return () => {
      clearTimeout(initialTimer);
      clearInterval(timer);
    };
  }, [loadPrinters, loadBambu, intervalMs]);

  const moonrakerEntries = Object.entries(printers ?? {}).filter(
    ([id, cfg]) => id !== "no_printers" && cfg.driver !== "bambu_ha"
  );
  const hasAnyPrinter =
    moonrakerEntries.length > 0 || bambuPrinters.length > 0;
  const spools = status?.spools ?? [];
  const printErrors = status?.print_errors ?? [];

  const onChanged = () => {
    refresh();
    loadBambu();
  };

  return (
    <div className="min-w-0 space-y-6">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Dashboard</h1>
          <p className="text-sm text-muted-foreground">
            Printer status and spool mapping
          </p>
        </div>
        <Badge
          variant="outline"
          className={
            connected
              ? "border-success/30 bg-success/10 text-success"
              : "border-warning/30 bg-warning/10 text-warning"
          }
        >
          {connected ? (
            <>
              <Wifi className="size-3" /> Live
            </>
          ) : (
            <>
              <WifiOff className="size-3" /> Reconnecting...
            </>
          )}
        </Badge>
      </header>

      {spoolmanConfigured === false ? (
        <Card className="border-primary/30 bg-primary/5">
          <CardContent className="flex flex-col items-center justify-center gap-5 py-14 text-center">
            <div className="flex size-14 items-center justify-center rounded-2xl border border-primary/30 bg-primary/10">
              <Database className="size-7 text-primary" />
            </div>
            <div className="mx-auto max-w-md space-y-2">
              <p className="text-lg font-semibold leading-none">
                Connect Spoolman
              </p>
              <p className="text-sm text-balance text-muted-foreground">
                Spoolman is the filament inventory at the heart of FilaBridge+ —
                it is essential for tracking spools and debiting filament usage.
                Set it up to get started.
              </p>
            </div>
            <Button asChild>
              <Link href="/settings?tab=spoolman">
                <Settings className="size-4" />
                Configure Spoolman
              </Link>
            </Button>
          </CardContent>
        </Card>
      ) : (
        spoolmanConfigured === true &&
        spoolmanOk === false && (
          <Card className="border-warning/40 bg-warning/5">
            <CardContent className="flex flex-col items-center justify-center gap-5 py-14 text-center">
              <div className="flex size-14 items-center justify-center rounded-2xl border border-warning/40 bg-warning/10">
                <CloudOff className="size-7 text-warning" />
              </div>
              <div className="mx-auto max-w-md space-y-2">
                <p className="text-lg font-semibold leading-none">
                  Spoolman is unreachable
                </p>
                <p className="text-sm text-balance text-muted-foreground">
                  FilaBridge+ needs a working Spoolman connection to track spools
                  and debit filament usage. Could not connect to the configured
                  URL — make sure Spoolman is running and the address is correct.
                </p>
              </div>
              <Button asChild>
                <Link href="/settings?tab=spoolman">
                  <Settings className="size-4" />
                  Check Spoolman settings
                </Link>
              </Button>
            </CardContent>
          </Card>
        )
      )}

      <PrintErrors errors={printErrors} onChanged={onChanged} />

      {printers === null ? (
        <div className="grid min-w-0 gap-4 md:grid-cols-2">
          <Skeleton className="h-64 rounded-xl" />
          <Skeleton className="h-64 rounded-xl" />
        </div>
      ) : !hasAnyPrinter ? (
        <Card className="border-dashed border-border bg-card/40">
          <CardContent className="flex flex-col items-center justify-center gap-5 py-14 text-center">
            <div className="flex size-14 items-center justify-center rounded-2xl border border-border bg-background/60">
              <Printer className="size-7 text-muted-foreground" />
            </div>
            <div className="mx-auto max-w-md space-y-2">
              <p className="text-lg font-semibold leading-none">
                Welcome to FilaBridge+
              </p>
              <p className="text-sm text-balance text-muted-foreground">
                No printers configured yet. Add your Snapmaker U1 (Moonraker) or
                Bambu Lab (Home Assistant) to start tracking filament usage
                automatically.
              </p>
            </div>
            <Button asChild>
              <Link href="/settings?tab=printers">
                <PlugZap className="size-4" />
                Configure printers
              </Link>
            </Button>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-4">
          {bambuError && bambuPrinters.length === 0 && (
            <Card className="border-warning/40 bg-warning/5">
              <CardContent className="py-6 text-center">
                <p className="text-sm text-balance text-muted-foreground">
                  <span className="font-medium text-foreground">Bambu:</span>{" "}
                  {bambuError}
                </p>
              </CardContent>
            </Card>
          )}
          <div className="grid min-w-0 gap-4 md:grid-cols-2">
            {moonrakerEntries.map(([printerId, cfg]) => (
              <PrinterCard
                key={printerId}
                printerId={printerId}
                config={cfg}
                data={status?.printers?.[printerId]}
                mappings={status?.toolhead_mappings?.[printerId]}
                spools={spools}
                spoolmanUrl={spoolmanUrl}
                onChanged={onChanged}
              />
            ))}
            {bambuPrinters.map((printer) => (
              <BambuPrinterCard
                key={printer.printer_id ?? printer.device_id}
                printer={printer}
                spools={spools}
                spoolmanUrl={spoolmanUrl}
                onChanged={onChanged}
              />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
