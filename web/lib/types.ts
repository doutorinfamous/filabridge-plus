// Types mirroring the Go backend JSON payloads.

export interface SpoolFilament {
  id: number;
  name: string;
  material: string;
  density: number;
  diameter: number;
  color_hex: string;
  settings_extruder_temp: number;
  settings_bed_temp: number;
  vendor?: { id: number; name: string } | null;
}

export interface Spool {
  id: number;
  name: string;
  brand: string;
  material: string;
  location: string;
  remaining_weight: number;
  used_weight: number;
  filament?: SpoolFilament | null;
  extra?: Record<string, unknown>;
}

export interface Filament {
  id: number;
  name: string;
  material: string;
  color_hex: string;
  diameter: number;
  density: number;
  settings_extruder_temp: number;
  settings_bed_temp: number;
  vendor?: { id: number; name: string } | null;
}

export interface PrinterData {
  name: string;
  state: string;
  job_name?: string;
  progress?: number;
  print_duration?: number;
  time_remaining?: number | null;
  current_layer?: number | null;
  total_layer?: number | null;
}

export interface ToolheadMapping {
  printer_id: string;
  printer_name: string;
  toolhead_id: number;
  spool_id: number;
  display_name?: string;
}

export interface PrintJobUsage {
  spool_id: number;
  toolhead_id?: number;
  tray_unique_id?: string;
  slot_name: string;
  grams: number;
}

export interface PrintJob {
  id: number;
  printer_id: string;
  printer_name: string;
  job_name: string;
  started_at?: string | null;
  finished_at?: string | null;
  status: "printing" | "completed" | "cancelled" | "failed";
  total_grams: number;
  usage: PrintJobUsage[];
}

export interface PrintHistoryResponse {
  jobs: PrintJob[];
  total: number;
  limit: number;
  offset: number;
}

export interface PrintError {
  id: string;
  printer_id?: string;
  printer_name: string;
  filename: string;
  job_name?: string;
  toolhead_id?: number;
  grams?: number;
  error: string;
  timestamp: string;
  acknowledged: boolean;
}

export interface StatusMessage {
  type: string;
  timestamp: string;
  printers: Record<string, PrinterData>;
  spools: Spool[];
  toolhead_mappings: Record<string, Record<number, ToolheadMapping>>;
  print_errors?: PrintError[];
}

export interface PrinterConfigInfo {
  name: string;
  model: string;
  driver: string;
  ip_address: string;
  api_key: string;
  toolheads: number;
  ha_prefix?: string;
  ha_device_id?: string;
  toolhead_names?: Record<number, string>;
}

export interface BambuTray {
  entity_id: string;
  unique_id: string;
  tray_number: number;
  ams_number: number;
  is_external: boolean;
  name?: string;
  color?: string;
  material?: string;
  remaining_weight?: number;
  display_name?: string;
  assigned_spool_id?: number | null;
}

export interface BambuAMS {
  entity_id: string;
  name: string;
  ams_number: number;
  trays: BambuTray[];
}

export interface BambuPrinter {
  entity_id: string;
  device_id: string;
  prefix: string;
  name: string;
  state?: string;
  job_name?: string;
  progress?: number;
  print_duration?: number;
  time_remaining?: number | null;
  current_layer?: number | null;
  total_layer?: number | null;
  ams_units: BambuAMS[] | null;
  external_spools: BambuTray[] | null;
  registered?: boolean;
  printer_id?: string;
}

export interface NfcUrlEntry {
  type: "spool" | "filament" | "location";
  url: string;
  qr_code_base64: string;
  // spool
  spool_id?: number;
  spool_name?: string;
  remaining_weight?: number;
  filament_id?: number;
  filament_name?: string;
  // shared
  material?: string;
  brand?: string;
  color_hex?: string;
  // location
  location_type?: "storage" | "ams_slot" | "toolhead";
  location_name?: string;
  display_name?: string;
  printer_name?: string;
  toolhead_display_name?: string;
  tray_unique_id?: string;
}

export interface NfcSessionSpool {
  id: number;
  name?: string;
  material?: string;
  brand?: string;
  color_hex?: string;
  remaining_weight?: number;
  location?: string;
}

export interface NfcSessionFilament {
  id: number;
  name?: string;
  material?: string;
  brand?: string;
  color_hex?: string;
  candidates: NfcSessionSpool[];
}

export interface NfcSelectSpoolSuccess {
  spool_id: number;
  spool_name?: string;
  spool_material?: string;
  spool_brand?: string;
  spool_color?: string;
  spool_weight?: number;
  location: string;
  location_type?: "storage" | "ams_slot" | "toolhead";
  printer?: string;
  toolhead?: string;
  slot?: string;
}

export interface NfcSelectSpoolResponse {
  completed: boolean;
  success?: NfcSelectSpoolSuccess;
  session?: NfcSessionStatus;
}

export interface NfcSessionLocation {
  name: string;
  display_name: string;
  location_type: "storage" | "ams_slot" | "toolhead";
  printer_name?: string;
  toolhead_display_name?: string;
}

export interface NfcSessionStatus {
  active: boolean;
  has_spool?: boolean;
  has_pending_filament?: boolean;
  has_location?: boolean;
  spool?: NfcSessionSpool;
  pending_filament?: NfcSessionFilament;
  location?: NfcSessionLocation;
  expires_at?: string;
}

export interface LocationEntry {
  name: string;
  type: string;
  is_virtual: boolean;
}

export interface HAConfig {
  ha_url: string;
  ha_token_set: boolean;
  filabridge_public_url: string;
}

export interface HAValidationCheck {
  entity_id: string;
  found: boolean;
  state?: string;
  required: boolean;
  hint?: string;
}

export interface HAValidation {
  prefix: string;
  package_file: string;
  all_ok: boolean;
  meter_missing: boolean;
  checks: HAValidationCheck[];
  fix_steps?: string[];
}

export interface DevDbTable {
  name: string;
  row_count: number;
}

export interface DevDbColumnSchema {
  name: string;
  type: string;
  not_null: boolean;
  primary_key: boolean;
  default_value?: string;
}

export interface DevDbTableData {
  table: string;
  schema?: DevDbColumnSchema[];
  columns: string[];
  rows: Record<string, unknown>[];
  total: number;
  limit: number;
  offset: number;
}
