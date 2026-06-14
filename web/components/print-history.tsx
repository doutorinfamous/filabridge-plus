"use client";

import * as React from "react";
import {
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  CircleDashed,
  History,
  Loader2,
  XCircle,
} from "lucide-react";
import { toast } from "sonner";

import { api, spoolLabel } from "@/lib/api";
import type { PrintJob, PrinterConfigInfo, Spool } from "@/lib/types";
import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";

const PAGE_SIZE = 25;
const ALL_PRINTERS = "__all__";

const statusConfig: Record<
  PrintJob["status"],
  { label: string; icon: React.ElementType; className: string }
> = {
  printing: {
    label: "Printing",
    icon: Loader2,
    className: "border-blue-500/40 bg-blue-500/10 text-blue-400",
  },
  completed: {
    label: "Completed",
    icon: CheckCircle2,
    className: "border-emerald-500/40 bg-emerald-500/10 text-emerald-400",
  },
  cancelled: {
    label: "Cancelled",
    icon: CircleDashed,
    className: "border-amber-500/40 bg-amber-500/10 text-amber-400",
  },
  failed: {
    label: "Failed",
    icon: XCircle,
    className: "border-red-500/40 bg-red-500/10 text-red-400",
  },
};

function formatDate(value?: string | null): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return date.toLocaleString("en-US", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function formatGrams(grams: number): string {
  return `${grams.toLocaleString("en-US", {
    minimumFractionDigits: 0,
    maximumFractionDigits: 2,
  })}g`;
}

function StatusBadge({ status }: { status: PrintJob["status"] }) {
  const config = statusConfig[status] ?? statusConfig.failed;
  const Icon = config.icon;
  return (
    <Badge variant="outline" className={cn("gap-1", config.className)}>
      <Icon
        className={cn("size-3", status === "printing" && "animate-spin")}
      />
      {config.label}
    </Badge>
  );
}

function JobCard({
  job,
  spoolsById,
}: {
  job: PrintJob;
  spoolsById: Map<number, Spool>;
}) {
  return (
    <Card className="border-border/70 bg-card/60 py-0">
      <CardContent className="p-4">
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div className="min-w-0">
            <p className="truncate font-medium" title={job.job_name}>
              {job.job_name || "(no file name)"}
            </p>
            <p className="text-xs text-muted-foreground">
              {job.printer_name} · {formatDate(job.finished_at ?? job.started_at)}
            </p>
          </div>
          <div className="flex items-center gap-2">
            {job.total_grams > 0 && (
              <span className="text-sm font-semibold tabular-nums">
                {formatGrams(job.total_grams)}
              </span>
            )}
            <StatusBadge status={job.status} />
          </div>
        </div>

        {job.usage.length > 0 && (
          <div className="mt-3 space-y-1 rounded-lg border border-border/60 bg-background/40 p-2">
            {job.usage.map((entry) => {
              const spool = spoolsById.get(entry.spool_id);
              return (
                <div
                  key={`${job.id}-${entry.slot_name}-${entry.spool_id}`}
                  className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 px-1 py-0.5 text-xs"
                >
                  <span className="text-muted-foreground">
                    {entry.slot_name}
                  </span>
                  <span className="min-w-0 flex-1 truncate text-right sm:text-left">
                    {spool ? spoolLabel(spool) : `Spool #${entry.spool_id}`}
                  </span>
                  <span className="font-medium tabular-nums">
                    {formatGrams(entry.grams)}
                  </span>
                </div>
              );
            })}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

export function PrintHistory() {
  const [jobs, setJobs] = React.useState<PrintJob[] | null>(null);
  const [total, setTotal] = React.useState(0);
  const [offset, setOffset] = React.useState(0);
  const [printerFilter, setPrinterFilter] = React.useState(ALL_PRINTERS);
  const [printers, setPrinters] = React.useState<
    { id: string; name: string }[]
  >([]);
  const [spoolsById, setSpoolsById] = React.useState<Map<number, Spool>>(
    new Map()
  );
  const [loading, setLoading] = React.useState(true);

  React.useEffect(() => {
    api
      .getPrinters()
      .then((res) => {
        const entries = Object.entries(
          res.printers ?? ({} as Record<string, PrinterConfigInfo>)
        ).map(([id, config]) => ({ id, name: config.name || id }));
        entries.sort((a, b) => a.name.localeCompare(b.name));
        setPrinters(entries);
      })
      .catch(() => setPrinters([]));

    api
      .getSpools()
      .then((spools) =>
        setSpoolsById(new Map((spools ?? []).map((s) => [s.id, s])))
      )
      .catch(() => setSpoolsById(new Map()));
  }, []);

  // Loading state is toggled by the callers (handlers/initial state); state
  // here only changes in async callbacks, keeping effects clean.
  const loadJobs = React.useCallback(
    (nextOffset: number, printerId: string) => {
      api
        .getPrintHistory({
          limit: PAGE_SIZE,
          offset: nextOffset,
          printerId: printerId === ALL_PRINTERS ? undefined : printerId,
        })
        .then((res) => {
          setJobs(res.jobs ?? []);
          setTotal(res.total ?? 0);
          setOffset(nextOffset);
        })
        .catch((error) => {
          setJobs([]);
          setTotal(0);
          toast.error(
            error instanceof Error
              ? error.message
              : "Failed to load history"
          );
        })
        .finally(() => setLoading(false));
    },
    []
  );

  React.useEffect(() => {
    loadJobs(0, printerFilter);
  }, [loadJobs, printerFilter]);

  const canPrev = offset > 0;
  const canNext = offset + PAGE_SIZE < total;

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Select
          value={printerFilter}
          onValueChange={(value) => {
            setLoading(true);
            setPrinterFilter(value);
          }}
        >
          <SelectTrigger className="w-56">
            <SelectValue placeholder="All printers" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL_PRINTERS}>All printers</SelectItem>
            {printers.map((printer) => (
              <SelectItem key={printer.id} value={printer.id}>
                {printer.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!canPrev || loading}
            onClick={() => {
              setLoading(true);
              loadJobs(offset - PAGE_SIZE, printerFilter);
            }}
          >
            <ChevronLeft className="size-4" />
            Previous
          </Button>
          <span className="text-xs text-muted-foreground">
            {total === 0
              ? "0"
              : `${offset + 1}–${Math.min(offset + PAGE_SIZE, total)} of ${total}`}
          </span>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!canNext || loading}
            onClick={() => {
              setLoading(true);
              loadJobs(offset + PAGE_SIZE, printerFilter);
            }}
          >
            Next
            <ChevronRight className="size-4" />
          </Button>
        </div>
      </div>

      {loading ? (
        <div className="space-y-3">
          {["a", "b", "c", "d"].map((id) => (
            <Skeleton key={id} className="h-24 rounded-xl" />
          ))}
        </div>
      ) : jobs == null || jobs.length === 0 ? (
        <Card className="border-border/70 bg-card/60">
          <CardContent className="flex flex-col items-center justify-center py-16 text-muted-foreground">
            <History className="mb-3 size-10 opacity-40" />
            <p className="text-sm font-medium">No prints recorded</p>
            <p className="text-xs">
              History appears here when prints are completed
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-3">
          {jobs.map((job) => (
            <JobCard key={job.id} job={job} spoolsById={spoolsById} />
          ))}
        </div>
      )}
    </div>
  );
}
