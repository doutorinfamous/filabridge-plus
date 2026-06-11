// Thin typed client for the FilaBridge API. All paths are relative — the
// Next.js server proxies /api/* to the Go backend, so a single port serves
// both the UI and the API.

import type {
  BambuPrinter,
  Filament,
  HAConfig,
  HAValidation,
  LocationEntry,
  NfcUrlEntry,
  PrintError,
  PrinterConfigInfo,
  Spool,
  StatusMessage,
} from "./types";

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: {
      ...(init?.body ? { "Content-Type": "application/json" } : {}),
      ...init?.headers,
    },
    cache: "no-store",
  });

  let data: unknown = null;
  const text = await res.text();
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = text;
    }
  }

  if (!res.ok) {
    const message =
      data && typeof data === "object" && "error" in data
        ? String((data as { error: unknown }).error)
        : res.statusText || `HTTP ${res.status}`;
    throw new ApiError(res.status, message);
  }

  return data as T;
}

export const api = {
  // Status / spools
  getStatus: () =>
    request<Omit<StatusMessage, "type" | "spools">>("/api/status"),
  getSpools: () => request<Spool[]>("/api/spools"),
  getFilaments: () => request<Filament[]>("/api/filaments"),
  getAvailableSpools: (params: {
    printerName?: string;
    toolheadId?: number;
    trayUniqueId?: string;
  }) => {
    const q = new URLSearchParams();
    if (params.trayUniqueId) {
      q.set("tray_unique_id", params.trayUniqueId);
    } else {
      q.set("printer_name", params.printerName ?? "");
      q.set("toolhead_id", String(params.toolheadId ?? 0));
    }
    return request<{ spools: Spool[] | null }>(`/api/available_spools?${q}`);
  },
  mapToolhead: (printerName: string, toolheadId: number, spoolId: number) =>
    request<{ message: string }>("/api/map_toolhead", {
      method: "POST",
      body: JSON.stringify({
        printer_name: printerName,
        toolhead_id: toolheadId,
        spool_id: spoolId,
      }),
    }),
  assignTray: (trayUniqueId: string, spoolId: number) =>
    request<{ success: boolean }>("/api/trays/assign", {
      method: "POST",
      body: JSON.stringify({ tray_unique_id: trayUniqueId, spool_id: spoolId }),
    }),

  // Spoolman
  testSpoolman: () =>
    request<{ connected: boolean }>("/api/spoolman/test"),

  // Config
  getConfig: () => request<Record<string, string>>("/api/config"),
  updateConfig: (config: Record<string, string>) =>
    request<{ message: string }>("/api/config", {
      method: "POST",
      body: JSON.stringify(config),
    }),
  getAutoAssign: () =>
    request<{ enabled: boolean; location: string }>(
      "/api/config/auto-assign-previous-spool"
    ),
  updateAutoAssign: (enabled: boolean, location: string) =>
    request<{ message: string }>("/api/config/auto-assign-previous-spool", {
      method: "PUT",
      body: JSON.stringify({ enabled, location }),
    }),

  // Printers
  getPrinters: () =>
    request<{ printers: Record<string, PrinterConfigInfo> }>("/api/printers"),
  detectPrinter: (ipAddress: string, apiKey: string) =>
    request<{ model: string; hostname: string; detected: boolean; warning?: string }>(
      "/api/detect_printer",
      {
        method: "POST",
        body: JSON.stringify({ ip_address: ipAddress, api_key: apiKey }),
      }
    ),
  addPrinter: (config: {
    name: string;
    model: string;
    ip_address: string;
    api_key: string;
    toolheads: number;
  }) =>
    request<{ printer_id: string }>("/api/printers", {
      method: "POST",
      body: JSON.stringify(config),
    }),
  updatePrinter: (
    printerId: string,
    config: {
      name: string;
      model: string;
      ip_address: string;
      api_key: string;
      toolheads: number;
    }
  ) =>
    request<{ message: string }>(`/api/printers/${printerId}`, {
      method: "PUT",
      body: JSON.stringify(config),
    }),
  deletePrinter: (printerId: string) =>
    request<{ message: string }>(`/api/printers/${printerId}`, {
      method: "DELETE",
    }),
  setToolheadName: (printerId: string, toolheadId: number, name: string) =>
    request<{ message: string }>(
      `/api/printers/${printerId}/toolheads/${toolheadId}`,
      { method: "PUT", body: JSON.stringify({ name }) }
    ),

  // Print errors
  getPrintErrors: () =>
    request<{ errors: PrintError[] | null }>("/api/print-errors"),
  acknowledgePrintError: (id: string) =>
    request<{ message: string }>(
      `/api/print-errors/${encodeURIComponent(id)}/acknowledge`,
      { method: "POST" }
    ),

  // Locations
  getLocations: () =>
    request<{ locations: LocationEntry[] | null; spoolman_url: string }>(
      "/api/locations"
    ),

  // NFC
  getNfcUrls: () =>
    request<{ urls: NfcUrlEntry[] | null; spoolman_url: string }>(
      "/api/nfc/urls"
    ),

  // Home Assistant / Bambu
  getHAConfig: () => request<HAConfig>("/api/ha/config"),
  updateHAConfig: (config: {
    ha_url?: string;
    ha_token?: string;
    filabridge_public_url?: string;
  }) =>
    request<{ success: boolean }>("/api/ha/config", {
      method: "POST",
      body: JSON.stringify(config),
    }),
  testHA: (haUrl?: string, haToken?: string) =>
    request<{ success: boolean; error?: string }>("/api/ha/test", {
      method: "POST",
      body: JSON.stringify({ ha_url: haUrl ?? "", ha_token: haToken ?? "" }),
    }),
  getBambuPrinters: () => request<BambuPrinter[]>("/api/ha/printers"),
  registerBambuPrinter: (printer: BambuPrinter) =>
    request<{ printer_id: string; success: boolean }>("/api/ha/printers", {
      method: "POST",
      body: JSON.stringify(printer),
    }),
  getHAAutomations: (printerId: string) =>
    request<{ yaml: string; webhook_url: string; filename: string }>(
      `/api/ha/automations/${printerId}`
    ),
  validateHA: (printerId: string) =>
    request<HAValidation>(`/api/ha/validate/${printerId}`),
};

export function spoolLabel(spool: Spool): string {
  const material = spool.material || "Material desconhecido";
  const brand = spool.brand || "Marca desconhecida";
  const name = spool.name || "Sem nome";
  return `[${spool.id}] ${material} · ${brand} · ${name}`;
}

export function formatRemainingWeight(spool: Spool): string | null {
  if (spool.remaining_weight == null) return null;
  return `${Math.round(spool.remaining_weight)}g`;
}

export function spoolColor(spool: Spool | null | undefined): string {
  const hex = spool?.filament?.color_hex;
  if (!hex) return "#71717a";
  return hex.startsWith("#") ? hex : `#${hex}`;
}

export function formatDuration(seconds?: number | null): string {
  if (seconds == null || seconds < 0) return "--";
  const total = Math.round(seconds);
  if (total <= 0) return "0s";
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}

export function progressPercent(progress?: number): number {
  if (progress == null) return 0;
  const pct = progress <= 1 ? progress * 100 : progress;
  return Math.min(100, Math.max(0, pct));
}
