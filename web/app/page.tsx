"use client";

import * as React from "react";
import Link from "next/link";
import { CloudOff, PlugZap, Printer, Wifi, WifiOff } from "lucide-react";

import { api } from "@/lib/api";
import type { BambuPrinter, PrinterConfigInfo } from "@/lib/types";
import { usePollInterval } from "@/lib/use-poll-interval";
import { useStatusSocket } from "@/lib/use-status-socket";
import {
  Alert,
  AlertDescription,
  AlertTitle,
} from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
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
      setBambuError(error instanceof Error ? error.message : "erro");
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
        .then((cfg) =>
          setSpoolmanUrl((cfg.spoolman_url ?? "").replace(/\/$/, ""))
        )
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
            Status das impressoras e mapeamento de spools
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
              <Wifi className="size-3" /> Tempo real
            </>
          ) : (
            <>
              <WifiOff className="size-3" /> Reconectando...
            </>
          )}
        </Badge>
      </header>

      {spoolmanOk === false && (
        <Alert className="border-warning/40 bg-warning/10 text-warning">
          <CloudOff className="size-4" />
          <AlertTitle>Spoolman inacessível</AlertTitle>
          <AlertDescription className="text-warning/90">
            Não foi possível conectar ao Spoolman. Verifique a URL em{" "}
            <Link href="/settings" className="underline underline-offset-2">
              Configurações
            </Link>
            .
          </AlertDescription>
        </Alert>
      )}

      <PrintErrors errors={printErrors} onChanged={onChanged} />

      {printers === null ? (
        <div className="grid min-w-0 gap-4 md:grid-cols-2">
          <Skeleton className="h-64 rounded-xl" />
          <Skeleton className="h-64 rounded-xl" />
        </div>
      ) : !hasAnyPrinter ? (
        <Card className="border-dashed border-border bg-card/40">
          <CardHeader className="items-center text-center">
            <div className="mx-auto flex size-14 items-center justify-center rounded-2xl border border-border bg-background/60">
              <Printer className="size-7 text-muted-foreground" />
            </div>
            <CardTitle className="text-lg">Bem-vindo ao FilaBridge</CardTitle>
            <CardDescription className="max-w-md">
              Nenhuma impressora configurada ainda. Adicione sua Snapmaker U1
              (Moonraker) ou Bambu Lab (Home Assistant) para começar a
              rastrear o uso de filamento automaticamente.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex justify-center pb-8">
            <Button asChild>
              <Link href="/settings?tab=printers">
                <PlugZap className="size-4" />
                Configurar impressoras
              </Link>
            </Button>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-4">
          {bambuError && bambuPrinters.length === 0 && (
            <p className="text-sm text-muted-foreground">Bambu: {bambuError}</p>
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
