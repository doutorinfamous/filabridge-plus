package core

import (
	"database/sql"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"filabridge/spoolman"
)

// newLegacyV2Database creates a database file with the pre-v3 schema
// (separate toolhead_mappings and bambu_trays tables) and sample data.
func newLegacyV2Database(t *testing.T) string {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("failed to open legacy db: %v", err)
	}
	defer db.Close()

	statements := []string{
		`CREATE TABLE toolhead_mappings (
			printer_id TEXT NOT NULL,
			toolhead_id INTEGER NOT NULL,
			display_name TEXT NOT NULL DEFAULT '',
			spool_id INTEGER,
			mapped_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (printer_id, toolhead_id)
		)`,
		`INSERT INTO toolhead_mappings (printer_id, toolhead_id, display_name, spool_id)
			VALUES ('printer1', 0, 'Hotend Esquerdo', 7)`,
		`INSERT INTO toolhead_mappings (printer_id, toolhead_id, display_name, spool_id)
			VALUES ('printer1', 1, '', NULL)`,
		`CREATE TABLE bambu_trays (
			printer_id TEXT NOT NULL,
			tray_unique_id TEXT PRIMARY KEY,
			entity_id TEXT NOT NULL,
			ams_number INTEGER NOT NULL DEFAULT 0,
			tray_number INTEGER NOT NULL DEFAULT 0,
			display_name TEXT NOT NULL,
			is_external INTEGER NOT NULL DEFAULT 0
		)`,
		`INSERT INTO bambu_trays VALUES ('bambu_x1c', 'tray_a', 'sensor.x1c_tray_1', 1, 1, 'X1C - AMS 1 Slot 1', 0)`,
		`INSERT INTO bambu_trays VALUES ('bambu_x1c', 'ext_a', 'sensor.x1c_external', 0, 0, 'X1C - External Spool', 1)`,
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("failed to seed legacy db: %v", err)
		}
	}
	return dbPath
}

func TestMigrateSchemaV3FromLegacyTables(t *testing.T) {
	dbPath := newLegacyV2Database(t)

	bridge, err := NewFilamentBridge(&Config{DBFile: dbPath})
	if err != nil {
		t.Fatalf("failed to create bridge over legacy db: %v", err)
	}
	t.Cleanup(func() { bridge.Close() })

	// Toolhead mapping migrated with spool and display name.
	spoolID, err := bridge.GetToolheadMapping("printer1", 0)
	if err != nil {
		t.Fatalf("GetToolheadMapping failed: %v", err)
	}
	if spoolID != 7 {
		t.Fatalf("expected spool 7 on toolhead 0, got %d", spoolID)
	}
	name, err := bridge.GetToolheadName("printer1", 0)
	if err != nil || name != "Hotend Esquerdo" {
		t.Fatalf("expected display name preserved, got %q (err %v)", name, err)
	}

	// Bambu trays migrated as tray slots.
	traySlot, err := bridge.GetSlot("tray_a")
	if err != nil || traySlot == nil {
		t.Fatalf("expected tray_a slot, got %v (err %v)", traySlot, err)
	}
	if traySlot.SlotType != SlotTypeAMSTray || traySlot.EntityID != "sensor.x1c_tray_1" ||
		traySlot.AMSNumber != 1 || traySlot.TrayNumber != 1 || traySlot.PrinterID != "bambu_x1c" {
		t.Fatalf("unexpected tray slot: %+v", traySlot)
	}
	extSlot, err := bridge.GetSlot("ext_a")
	if err != nil || extSlot == nil {
		t.Fatalf("expected ext_a slot, got %v (err %v)", extSlot, err)
	}
	if extSlot.SlotType != SlotTypeExternal {
		t.Fatalf("expected external slot type, got %q", extSlot.SlotType)
	}

	// Old tables dropped.
	for _, table := range []string{"toolhead_mappings", "bambu_trays"} {
		exists, err := bridge.tableExists(table)
		if err != nil {
			t.Fatalf("tableExists(%s) failed: %v", table, err)
		}
		if exists {
			t.Fatalf("expected table %s to be dropped", table)
		}
	}

	// Tray spool backfill flagged as pending.
	flag, err := bridge.GetConfigValue(ConfigKeySlotsTrayBackfillDone)
	if err != nil || flag != "false" {
		t.Fatalf("expected backfill flag 'false', got %q (err %v)", flag, err)
	}
}

func TestBackfillTraySpoolAssignments(t *testing.T) {
	dbPath := newLegacyV2Database(t)

	store := newSpoolStore(spoolman.Spool{
		ID:              3,
		RemainingWeight: 500,
		Extra: map[string]interface{}{
			spoolman.ExtraFieldActiveTray: "tray_a",
		},
	})
	server := httptest.NewServer(store.handler())
	t.Cleanup(server.Close)

	bridge, err := NewFilamentBridge(&Config{DBFile: dbPath, SpoolmanURL: server.URL})
	if err != nil {
		t.Fatalf("failed to create bridge: %v", err)
	}
	t.Cleanup(func() { bridge.Close() })

	if err := bridge.BackfillTraySpoolAssignments(); err != nil {
		t.Fatalf("BackfillTraySpoolAssignments failed: %v", err)
	}

	slot, err := bridge.GetSlot("tray_a")
	if err != nil || slot == nil {
		t.Fatalf("expected tray_a slot, got %v (err %v)", slot, err)
	}
	if slot.SpoolID != 3 {
		t.Fatalf("expected spool 3 backfilled on tray_a, got %d", slot.SpoolID)
	}

	flag, err := bridge.GetConfigValue(ConfigKeySlotsTrayBackfillDone)
	if err != nil || flag != "true" {
		t.Fatalf("expected backfill flag 'true', got %q (err %v)", flag, err)
	}
}
