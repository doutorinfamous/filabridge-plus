import assert from "node:assert/strict";
import { describe, it } from "node:test";

import type { BambuTray, Spool } from "./types";
import { findSpoolForBambuTray } from "./bambu-tray-spool";

function makeSpool(id: number, overrides: Partial<Spool> = {}): Spool {
  return {
    id,
    name: `Spool ${id}`,
    brand: "B",
    material: "PLA",
    location: "",
    remaining_weight: 500,
    used_weight: 0,
    ...overrides,
  };
}

const tray: BambuTray = {
  unique_id: "tray_unique_4",
  entity_id: "sensor.bambu_tray_4",
  tray_number: 4,
  ams_number: 1,
  is_external: false,
  assigned_spool_id: 1,
};

describe("findSpoolForBambuTray", () => {
  it("returns spool matching assigned_spool_id", () => {
    const spools: Spool[] = [makeSpool(1), makeSpool(2)];
    const match = findSpoolForBambuTray(tray, spools);
    assert.equal(match?.id, 1);
  });

  it("returns null when no spool is assigned to the tray", () => {
    const emptyTray: BambuTray = { ...tray, assigned_spool_id: null };
    assert.equal(findSpoolForBambuTray(emptyTray, [makeSpool(1)]), null);
  });

  it("returns null when the assigned spool is not in the list", () => {
    const spools: Spool[] = [makeSpool(2)];
    assert.equal(findSpoolForBambuTray(tray, spools), null);
  });
});
