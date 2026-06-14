import type { BambuTray, Spool } from "@/lib/types";

/**
 * Finds the spool assigned to a Bambu tray. The backend resolves the
 * assignment from the unified printer_slots table and ships it on the tray
 * payload as assigned_spool_id.
 */
export function findSpoolForBambuTray(
  tray: BambuTray,
  spools: Spool[]
): Spool | null {
  if (!tray.assigned_spool_id) return null;
  return spools.find((spool) => spool.id === tray.assigned_spool_id) ?? null;
}
