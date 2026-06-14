"use client";

import * as React from "react";
import { Check, ChevronsUpDown, CircleOff, Loader2 } from "lucide-react";

import {
  formatRemainingWeight,
  spoolColor,
  spoolLabel,
} from "@/lib/api";
import type { Spool } from "@/lib/types";
import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";

export function SpoolDot({
  color,
  className,
}: {
  color: string;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-block size-3.5 shrink-0 rounded-full ring-1 ring-white/20",
        className
      )}
      style={{ backgroundColor: color }}
    />
  );
}

export function SpoolWeightBadge({
  spool,
  className,
}: {
  spool: Spool;
  className?: string;
}) {
  const weight = formatRemainingWeight(spool);
  if (!weight) return null;

  return (
    <Badge
      variant="secondary"
      className={cn(
        "shrink-0 rounded-full px-2 py-0 text-[11px] font-medium tabular-nums",
        className
      )}
    >
      {weight}
    </Badge>
  );
}

interface SpoolSelectProps {
  /** Spool currently assigned (from the full spool list), or null. */
  currentSpool: Spool | null;
  /** Loads the list of assignable spools when the dropdown opens. */
  loadAvailable: () => Promise<Spool[]>;
  /** Called with the chosen spool id, or 0 to clear the slot. */
  onSelect: (spoolId: number) => Promise<void>;
  disabled?: boolean;
}

export function SpoolSelect({
  currentSpool,
  loadAvailable,
  onSelect,
  disabled,
}: SpoolSelectProps) {
  const [open, setOpen] = React.useState(false);
  const [loading, setLoading] = React.useState(false);
  const [saving, setSaving] = React.useState(false);
  const [options, setOptions] = React.useState<Spool[]>([]);

  const handleOpenChange = async (next: boolean) => {
    setOpen(next);
    if (next) {
      setLoading(true);
      try {
        const available = await loadAvailable();
        const merged = [...available];
        if (
          currentSpool &&
          !merged.some((spool) => spool.id === currentSpool.id)
        ) {
          merged.unshift(currentSpool);
        }
        setOptions(merged);
      } catch {
        setOptions(currentSpool ? [currentSpool] : []);
      } finally {
        setLoading(false);
      }
    }
  };

  const pick = async (spoolId: number) => {
    setOpen(false);
    if (spoolId === (currentSpool?.id ?? 0)) return;
    setSaving(true);
    try {
      await onSelect(spoolId);
    } finally {
      setSaving(false);
    }
  };

  const spoolSearchValue = (spool: Spool) => {
    const weight = formatRemainingWeight(spool);
    return `${spool.id} ${spoolLabel(spool)}${weight ? ` ${weight}` : ""}`;
  };

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          disabled={disabled || saving}
          className="h-9 w-full justify-between gap-2 bg-background/40 font-normal"
        >
          <span className="flex min-w-0 flex-1 items-center gap-2">
            {currentSpool ? (
              <>
                <SpoolDot color={spoolColor(currentSpool)} />
                <span className="truncate text-sm">
                  {spoolLabel(currentSpool)}
                </span>
              </>
            ) : (
              <>
                <CircleOff className="size-3.5 text-muted-foreground" />
                <span className="text-sm text-muted-foreground">Empty</span>
              </>
            )}
          </span>
          <span className="flex shrink-0 items-center gap-1.5">
            {currentSpool && <SpoolWeightBadge spool={currentSpool} />}
            {saving ? (
              <Loader2 className="size-4 animate-spin opacity-50" />
            ) : (
              <ChevronsUpDown className="size-4 opacity-50" />
            )}
          </span>
        </Button>
      </PopoverTrigger>
      <PopoverContent
        className="w-(--radix-popover-trigger-width) min-w-72 p-0"
        align="start"
      >
        <Command>
          <CommandInput placeholder="Search spools..." />
          <CommandList>
            <CommandEmpty>
              {loading ? "Loading..." : "No spools found"}
            </CommandEmpty>
            <CommandGroup>
              <CommandItem value="__empty__ empty" onSelect={() => pick(0)}>
                <CircleOff className="size-3.5 text-muted-foreground" />
                <span>Empty (remove spool)</span>
                {!currentSpool && <Check className="ml-auto size-4" />}
              </CommandItem>
              {options.map((spool) => (
                <CommandItem
                  key={spool.id}
                  value={spoolSearchValue(spool)}
                  onSelect={() => pick(spool.id)}
                >
                  <SpoolDot color={spoolColor(spool)} />
                  <span className="min-w-0 flex-1 truncate">
                    {spoolLabel(spool)}
                  </span>
                  <SpoolWeightBadge spool={spool} />
                  {currentSpool?.id === spool.id && (
                    <Check className="size-4 shrink-0" />
                  )}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
