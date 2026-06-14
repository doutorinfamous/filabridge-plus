"use client";

import * as React from "react";
import Link from "next/link";
import {
  AlertTriangle,
  Boxes,
  CheckCircle2,
  ChevronRight,
  Download,
  Loader2,
  Pencil,
  Plus,
  Printer,
  Tags,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/api";
import type { BambuPrinter, PrinterConfigInfo } from "@/lib/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";

interface PrinterFormState {
  name: string;
  ip_address: string;
  api_key: string;
  model: string;
  toolheads: number;
}

const emptyForm: PrinterFormState = {
  name: "",
  ip_address: "",
  api_key: "",
  model: "Snapmaker U1",
  toolheads: 4,
};

function PrinterFormFields({
  form,
  setForm,
}: {
  form: PrinterFormState;
  setForm: React.Dispatch<React.SetStateAction<PrinterFormState>>;
}) {
  return (
    <div className="space-y-4">
      <div className="space-y-1.5">
        <Label htmlFor="printer-name">Name *</Label>
        <Input
          id="printer-name"
          value={form.name}
          onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
          placeholder="e.g. Snapmaker U1"
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="printer-ip">Hostname or IP *</Label>
        <Input
          id="printer-ip"
          value={form.ip_address}
          onChange={(e) =>
            setForm((f) => ({ ...f, ip_address: e.target.value }))
          }
          placeholder="192.168.1.100 or printer.local"
        />
        <p className="text-xs text-muted-foreground">
          Moonraker instance address for the Snapmaker U1
        </p>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="printer-key">API Key (optional)</Label>
        <Input
          id="printer-key"
          type="password"
          value={form.api_key}
          onChange={(e) => setForm((f) => ({ ...f, api_key: e.target.value }))}
          placeholder="Only if Moonraker requires authentication"
        />
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label>Model</Label>
          <Input value="Snapmaker U1" disabled readOnly />
          <p className="text-xs text-muted-foreground">
            Only the Snapmaker U1 is supported at the moment
          </p>
        </div>
        <div className="space-y-1.5">
          <Label>Toolheads</Label>
          <Select
            value={String(form.toolheads)}
            onValueChange={(v) =>
              setForm((f) => ({ ...f, toolheads: Number(v) }))
            }
          >
            <SelectTrigger className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {[1, 2, 3, 4, 5].map((n) => (
                <SelectItem key={n} value={String(n)}>
                  {n} toolhead{n > 1 ? "s" : ""}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>
    </div>
  );
}

function ToolheadNamesEditor({
  printerId,
  printer,
  onSaved,
}: {
  printerId: string;
  printer: PrinterConfigInfo;
  onSaved: () => void;
}) {
  const [names, setNames] = React.useState<Record<number, string>>(
    printer.toolhead_names ?? {}
  );
  const [saving, setSaving] = React.useState(false);

  const save = async () => {
    setSaving(true);
    try {
      const updates = Object.entries(names).filter(([id, name]) => {
        const original =
          printer.toolhead_names?.[Number(id)] ?? `Toolhead ${Number(id) + 1}`;
        return name.trim() !== "" && name.trim() !== original;
      });
      if (updates.length === 0) {
        toast.info("Nothing to save");
        return;
      }
      await Promise.all(
        updates.map(([id, name]) =>
          api.setToolheadName(printerId, Number(id), name.trim())
        )
      );
      toast.success("Toolhead names saved");
      onSaved();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to save");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-3 rounded-lg border border-border/60 bg-background/40 p-3">
      {Array.from({ length: printer.toolheads }, (_, i) => i).map((id) => (
        <div key={id} className="flex items-center gap-3">
          <span className="w-24 shrink-0 text-sm text-muted-foreground">
            Toolhead {id + 1}
          </span>
          <Input
            value={names[id] ?? `Toolhead ${id + 1}`}
            onChange={(e) =>
              setNames((n) => ({ ...n, [id]: e.target.value }))
            }
            className="h-8"
          />
        </div>
      ))}
      <Button size="sm" onClick={save} disabled={saving}>
        {saving && <Loader2 className="size-3.5 animate-spin" />}
        Save names
      </Button>
    </div>
  );
}

export function PrintersSettings() {
  const [printers, setPrinters] = React.useState<Record<
    string,
    PrinterConfigInfo
  > | null>(null);
  const [addOpen, setAddOpen] = React.useState(false);
  const [addBusy, setAddBusy] = React.useState(false);
  const [addForm, setAddForm] = React.useState<PrinterFormState>(emptyForm);

  const [editId, setEditId] = React.useState<string | null>(null);
  const [editBusy, setEditBusy] = React.useState(false);
  const [editForm, setEditForm] = React.useState<PrinterFormState>(emptyForm);

  const [bambuOpen, setBambuOpen] = React.useState(false);
  const [bambuList, setBambuList] = React.useState<BambuPrinter[] | null>(null);
  const [bambuError, setBambuError] = React.useState<string | null>(null);

  const [chooserOpen, setChooserOpen] = React.useState(false);
  const [haConfigured, setHaConfigured] = React.useState<boolean | null>(null);

  const [renamingId, setRenamingId] = React.useState<string | null>(null);

  const load = React.useCallback(async () => {
    try {
      const res = await api.getPrinters();
      const entries = Object.fromEntries(
        Object.entries(res.printers ?? {}).filter(([id]) => id !== "no_printers")
      );
      setPrinters(entries);
    } catch {
      setPrinters({});
      toast.error("Failed to load printers");
    }
  }, []);

  React.useEffect(() => {
    const timer = setTimeout(load, 0);
    return () => clearTimeout(timer);
  }, [load]);

  const addPrinter = async () => {
    if (!addForm.name.trim() || !addForm.ip_address.trim()) {
      toast.error("Enter name and address");
      return;
    }
    setAddBusy(true);
    try {
      // Detect the model first (non-blocking if the printer is offline)
      let model = addForm.model;
      try {
        const detection = await api.detectPrinter(
          addForm.ip_address.trim(),
          addForm.api_key
        );
        if (detection.detected && detection.model) model = detection.model;
      } catch {
        // keep selected model
      }
      await api.addPrinter({
        name: addForm.name.trim(),
        model,
        ip_address: addForm.ip_address.trim(),
        api_key: addForm.api_key,
        toolheads: addForm.toolheads,
      });
      toast.success("Printer added");
      setAddOpen(false);
      setAddForm(emptyForm);
      load();
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to add printer"
      );
    } finally {
      setAddBusy(false);
    }
  };

  const openEdit = (printerId: string, printer: PrinterConfigInfo) => {
    setEditId(printerId);
    setEditForm({
      name: printer.name,
      ip_address: printer.ip_address,
      api_key: printer.api_key,
      model: printer.model || "Snapmaker U1",
      toolheads: printer.toolheads || 4,
    });
  };

  const saveEdit = async () => {
    if (!editId) return;
    setEditBusy(true);
    try {
      await api.updatePrinter(editId, {
        name: editForm.name.trim(),
        model: editForm.model,
        ip_address: editForm.ip_address.trim(),
        api_key: editForm.api_key,
        toolheads: editForm.toolheads,
      });
      toast.success("Printer updated");
      setEditId(null);
      load();
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to update printer"
      );
    } finally {
      setEditBusy(false);
    }
  };

  const remove = async (printerId: string, name: string) => {
    if (!confirm(`Remove printer "${name}"?`)) return;
    try {
      await api.deletePrinter(printerId);
      toast.success("Printer removed");
      load();
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to remove printer"
      );
    }
  };

  const openChooser = () => {
    setChooserOpen(true);
    setHaConfigured(null);
    api
      .getHAConfig()
      .then((cfg) => setHaConfigured(Boolean(cfg.ha_url) && cfg.ha_token_set))
      .catch(() => setHaConfigured(false));
  };

  const openBambuDiscovery = async () => {
    setBambuOpen(true);
    setBambuList(null);
    setBambuError(null);
    try {
      const list = await api.getBambuPrinters();
      setBambuList((list ?? []).filter((p) => !p.registered));
    } catch (error) {
      setBambuError(
        error instanceof Error
          ? error.message
          : "Discovery failed — configure Home Assistant first"
      );
      setBambuList([]);
    }
  };

  const registerBambu = async (printer: BambuPrinter) => {
    try {
      await api.registerBambuPrinter(printer);
      toast.success(
        "Bambu printer registered! Generate the HA package and restart Home Assistant."
      );
      setBambuOpen(false);
      load();
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to register printer"
      );
    }
  };

  const downloadHAConfig = async (printerId: string) => {
    try {
      const data = await api.getHAAutomations(printerId);
      const blob = new Blob([data.yaml], { type: "text/yaml" });
      const a = document.createElement("a");
      a.href = URL.createObjectURL(blob);
      a.download = data.filename || "filabridge_ha.yaml";
      a.click();
      URL.revokeObjectURL(a.href);
      toast.success(
        `YAML downloaded. Save to config/packages/${data.filename} and fully restart HA. Webhook: ${data.webhook_url}`,
        { duration: 12000 }
      );
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to generate configuration"
      );
    }
  };

  const validateHA = async (printerId: string) => {
    try {
      const data = await api.validateHA(printerId);
      if (data.all_ok) {
        toast.success("Home Assistant: all 4 FilaBridge+ entities are OK");
        return;
      }
      const missing = data.checks
        .filter((c) => !c.found)
        .map((c) => c.entity_id)
        .join(", ");
      toast.error(
        `Missing entities in HA: ${missing}. Reinstall package ${data.package_file} and restart HA.`,
        { duration: 12000 }
      );
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to validate HA"
      );
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap gap-2">
        <Button onClick={openChooser}>
          <Plus className="size-4" /> Add printer
        </Button>
      </div>

      {printers === null ? (
        <div className="grid gap-4 md:grid-cols-2">
          <Skeleton className="h-40 rounded-xl" />
          <Skeleton className="h-40 rounded-xl" />
        </div>
      ) : Object.keys(printers).length === 0 ? (
        <Card className="border-dashed bg-card/40">
          <CardContent className="py-10 text-center text-sm text-muted-foreground">
            No printers configured. Add one above to get started.
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 md:grid-cols-2">
          {Object.entries(printers).map(([printerId, printer]) => {
            const isBambu = printer.driver === "bambu_ha";
            return (
              <Card key={printerId} className="border-border/70 bg-card/60">
                <CardHeader className="flex flex-row items-start justify-between gap-2 space-y-0">
                  <div className="flex items-center gap-3">
                    <div className="flex size-9 items-center justify-center rounded-lg border border-border/70 bg-background/60">
                      {isBambu ? (
                        <Boxes className="size-4.5 text-emerald-400" />
                      ) : (
                        <Printer className="size-4.5 text-muted-foreground" />
                      )}
                    </div>
                    <div>
                      <CardTitle className="text-base">{printer.name}</CardTitle>
                      <CardDescription>
                        {isBambu
                          ? `ha-bambulab · prefix ${printer.ha_prefix || "?"}`
                          : `${printer.model || "Moonraker"} · ${printer.ip_address}`}
                      </CardDescription>
                    </div>
                  </div>
                  <Badge variant="secondary">
                    {isBambu
                      ? "Bambu HA"
                      : `${printer.toolheads} toolhead${printer.toolheads > 1 ? "s" : ""}`}
                  </Badge>
                </CardHeader>
                <CardContent className="space-y-3">
                  <div className="flex flex-wrap gap-2">
                    {isBambu ? (
                      <>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => downloadHAConfig(printerId)}
                        >
                          <Download className="size-3.5" /> HA package
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => validateHA(printerId)}
                        >
                          <CheckCircle2 className="size-3.5" /> Validate HA
                        </Button>
                      </>
                    ) : (
                      <>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => openEdit(printerId, printer)}
                        >
                          <Pencil className="size-3.5" /> Edit
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() =>
                            setRenamingId((id) =>
                              id === printerId ? null : printerId
                            )
                          }
                        >
                          <Tags className="size-3.5" /> Toolheads
                        </Button>
                      </>
                    )}
                    <Button
                      variant="outline"
                      size="sm"
                      className="text-destructive hover:text-destructive"
                      onClick={() => remove(printerId, printer.name)}
                    >
                      <Trash2 className="size-3.5" /> Remove
                    </Button>
                  </div>
                  {!isBambu && renamingId === printerId && (
                    <ToolheadNamesEditor
                      printerId={printerId}
                      printer={printer}
                      onSaved={() => {
                        setRenamingId(null);
                        load();
                      }}
                    />
                  )}
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}

      {/* Choose printer type */}
      <Dialog open={chooserOpen} onOpenChange={setChooserOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add printer</DialogTitle>
            <DialogDescription>
              Choose the type of printer you want to add
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <button
              type="button"
              onClick={() => {
                setChooserOpen(false);
                setAddForm(emptyForm);
                setAddOpen(true);
              }}
              className="flex w-full items-center justify-between rounded-lg border border-border bg-background/50 px-4 py-3 text-left transition-colors hover:bg-accent"
            >
              <span className="flex items-center gap-3">
                <span className="flex size-9 shrink-0 items-center justify-center rounded-lg border border-border/70 bg-background/60">
                  <Printer className="size-4.5 text-muted-foreground" />
                </span>
                <span>
                  <span className="block text-sm font-medium">
                    Snapmaker U1
                  </span>
                  <span className="block text-xs text-muted-foreground">
                    Connected via Moonraker
                  </span>
                </span>
              </span>
              <ChevronRight className="size-4 text-muted-foreground" />
            </button>

            <button
              type="button"
              onClick={() => {
                if (!haConfigured) return;
                setChooserOpen(false);
                openBambuDiscovery();
              }}
              disabled={haConfigured !== true}
              className="flex w-full items-center justify-between rounded-lg border border-border bg-background/50 px-4 py-3 text-left transition-colors hover:bg-accent disabled:cursor-not-allowed disabled:opacity-70 disabled:hover:bg-background/50"
            >
              <span className="flex items-center gap-3">
                <span className="flex size-9 shrink-0 items-center justify-center rounded-lg border border-border/70 bg-background/60">
                  <Boxes className="size-4.5 text-emerald-400" />
                </span>
                <span>
                  <span className="block text-sm font-medium">Bambu Lab</span>
                  <span className="block text-xs text-muted-foreground">
                    Connected via Home Assistant (ha-bambulab)
                  </span>
                </span>
              </span>
              {haConfigured === null ? (
                <Loader2 className="size-4 animate-spin text-muted-foreground" />
              ) : haConfigured ? (
                <span className="flex items-center gap-1.5 text-xs text-emerald-400">
                  <CheckCircle2 className="size-3.5" /> HA configured
                </span>
              ) : (
                <ChevronRight className="size-4 text-muted-foreground" />
              )}
            </button>
            {haConfigured === false && (
              <div className="flex items-start gap-2 rounded-lg border border-warning/40 bg-warning/10 px-3 py-2.5 text-xs text-warning">
                <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
                <span>
                  Home Assistant is not configured yet — it is required for
                  Bambu Lab printers.{" "}
                  <Link
                    href="/settings?tab=home-assistant"
                    className="font-medium underline underline-offset-2"
                    onClick={() => setChooserOpen(false)}
                  >
                    Configure Home Assistant
                  </Link>
                </span>
              </div>
            )}
          </div>
        </DialogContent>
      </Dialog>

      {/* Add Moonraker printer */}
      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add Snapmaker U1</DialogTitle>
            <DialogDescription>
              Connects to the printer through its Moonraker instance
            </DialogDescription>
          </DialogHeader>
          <PrinterFormFields form={addForm} setForm={setAddForm} />
          <DialogFooter>
            <Button variant="outline" onClick={() => setAddOpen(false)}>
              Cancel
            </Button>
            <Button onClick={addPrinter} disabled={addBusy}>
              {addBusy && <Loader2 className="size-4 animate-spin" />}
              {addBusy ? "Detecting model..." : "Add"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Edit printer */}
      <Dialog open={editId !== null} onOpenChange={(o) => !o && setEditId(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Edit printer</DialogTitle>
          </DialogHeader>
          <PrinterFormFields form={editForm} setForm={setEditForm} />
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditId(null)}>
              Cancel
            </Button>
            <Button onClick={saveEdit} disabled={editBusy}>
              {editBusy && <Loader2 className="size-4 animate-spin" />}
              Save
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Bambu discovery */}
      <Dialog open={bambuOpen} onOpenChange={setBambuOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add Bambu Lab (Home Assistant)</DialogTitle>
            <DialogDescription>
              Printers discovered via ha-bambulab. Configure HA URL and token
              on the{" "}
              <Link
                href="/settings?tab=home-assistant"
                className="underline underline-offset-2"
                onClick={() => setBambuOpen(false)}
              >
                Home Assistant tab
              </Link>{" "}
              first.
            </DialogDescription>
          </DialogHeader>
          <Separator />
          {bambuList === null ? (
            <div className="flex items-center gap-2 py-6 text-sm text-muted-foreground">
              <Loader2 className="size-4 animate-spin" /> Discovering
              printers...
            </div>
          ) : bambuError ? (
            <p className="py-4 text-sm text-destructive">{bambuError}</p>
          ) : bambuList.length === 0 ? (
            <p className="py-4 text-sm text-muted-foreground">
              No unregistered Bambu printers found in Home Assistant.
            </p>
          ) : (
            <div className="space-y-2">
              {bambuList.map((printer) => (
                <button
                  type="button"
                  key={printer.device_id || printer.prefix}
                  onClick={() => registerBambu(printer)}
                  className="flex w-full items-center justify-between rounded-lg border border-border bg-background/50 px-4 py-3 text-left transition-colors hover:bg-accent"
                >
                  <span>
                    <span className="block text-sm font-medium">
                      {printer.name}
                    </span>
                    <span className="block text-xs text-muted-foreground">
                      {printer.prefix}
                    </span>
                  </span>
                  <Plus className="size-4 text-muted-foreground" />
                </button>
              ))}
            </div>
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}
