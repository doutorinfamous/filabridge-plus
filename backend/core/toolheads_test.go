package core

import (
	"testing"
)

func saveMoonrakerPrinter(t *testing.T, bridge *FilamentBridge, printerID, name string, toolheads int) {
	t.Helper()
	if err := bridge.SavePrinterConfig(printerID, PrinterConfig{
		Name:      name,
		Driver:    DriverMoonraker,
		Toolheads: toolheads,
	}); err != nil {
		t.Fatalf("SavePrinterConfig failed: %v", err)
	}
}

func TestSyncToolheadSlotsCreatesAllDisplayNames(t *testing.T) {
	bridge := newTestBridge(t)
	printerID := "printer_snapmaker"
	saveMoonrakerPrinter(t, bridge, printerID, "Snapmaker U1", 4)

	if err := bridge.SyncToolheadSlots(printerID, 4); err != nil {
		t.Fatalf("SyncToolheadSlots failed: %v", err)
	}

	for toolheadID := 0; toolheadID < 4; toolheadID++ {
		name, err := bridge.GetToolheadName(printerID, toolheadID)
		if err != nil {
			t.Fatalf("GetToolheadName(%d) failed: %v", toolheadID, err)
		}
		expected := DefaultToolheadDisplayName(toolheadID)
		if name != expected {
			t.Fatalf("toolhead %d: expected display name %q, got %q", toolheadID, expected, name)
		}
	}

	var count int
	if err := bridge.DB.QueryRow(
		"SELECT COUNT(*) FROM printer_slots WHERE printer_id = ? AND slot_type = ? AND display_name != ''",
		printerID, SlotTypeToolhead,
	).Scan(&count); err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if count != 4 {
		t.Fatalf("expected 4 toolhead slots with display_name, got %d", count)
	}
}

func TestSyncToolheadSlotsRemovesExcessToolheads(t *testing.T) {
	bridge := newTestBridge(t)
	printerID := "printer_snapmaker"
	saveMoonrakerPrinter(t, bridge, printerID, "Snapmaker U1", 4)

	if err := bridge.SyncToolheadSlots(printerID, 4); err != nil {
		t.Fatalf("SyncToolheadSlots(4) failed: %v", err)
	}
	if err := bridge.SyncToolheadSlots(printerID, 2); err != nil {
		t.Fatalf("SyncToolheadSlots(2) failed: %v", err)
	}

	var count int
	if err := bridge.DB.QueryRow(
		"SELECT COUNT(*) FROM printer_slots WHERE printer_id = ? AND slot_type = ?",
		printerID, SlotTypeToolhead,
	).Scan(&count); err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 toolhead slots after shrink, got %d", count)
	}
}

func TestSyncToolheadSlotsPreservesCustomName(t *testing.T) {
	bridge := newTestBridge(t)
	printerID := "printer_snapmaker"
	saveMoonrakerPrinter(t, bridge, printerID, "Snapmaker U1", 2)

	if err := bridge.SetToolheadName(printerID, 0, "Black"); err != nil {
		t.Fatalf("SetToolheadName failed: %v", err)
	}
	if err := bridge.SyncToolheadSlots(printerID, 2); err != nil {
		t.Fatalf("SyncToolheadSlots failed: %v", err)
	}

	name, err := bridge.GetToolheadName(printerID, 0)
	if err != nil {
		t.Fatalf("GetToolheadName failed: %v", err)
	}
	if name != "Black" {
		t.Fatalf("expected custom name preserved, got %q", name)
	}
}

func TestGenerateToolheadNFCURLsIncludesEmptySlots(t *testing.T) {
	bridge := newTestBridge(t)
	printerID := "printer_snapmaker"
	saveMoonrakerPrinter(t, bridge, printerID, "Snapmaker U1", 4)

	urls, err := GenerateToolheadNFCURLs(bridge, "http://localhost:5000")
	if err != nil {
		t.Fatalf("GenerateToolheadNFCURLs failed: %v", err)
	}

	toolheadEntries := 0
	for _, entry := range urls {
		if entry["location_type"] != "toolhead" {
			continue
		}
		if entry["printer_id"] != printerID {
			continue
		}
		toolheadEntries++
		if entry["qr_code_base64"] == "" {
			t.Fatal("expected QR code for toolhead entry")
		}
	}
	if toolheadEntries != 4 {
		t.Fatalf("expected 4 toolhead NFC URLs, got %d", toolheadEntries)
	}
}

func TestSetToolheadMappingPreservesDisplayName(t *testing.T) {
	bridge := newTestBridge(t)
	printerID := "printer_snapmaker"
	saveMoonrakerPrinter(t, bridge, printerID, "Snapmaker U1", 2)

	if err := bridge.SetToolheadName(printerID, 0, "Red"); err != nil {
		t.Fatalf("SetToolheadName failed: %v", err)
	}
	if err := bridge.SetToolheadMapping(printerID, 0, 42); err != nil {
		t.Fatalf("SetToolheadMapping failed: %v", err)
	}

	name, err := bridge.GetToolheadName(printerID, 0)
	if err != nil {
		t.Fatalf("GetToolheadName failed: %v", err)
	}
	if name != "Red" {
		t.Fatalf("expected custom name preserved after mapping, got %q", name)
	}

	mapped, err := bridge.GetToolheadMapping(printerID, 0)
	if err != nil {
		t.Fatalf("GetToolheadMapping failed: %v", err)
	}
	if mapped != 42 {
		t.Fatalf("expected spool 42 mapped, got %d", mapped)
	}
}

func TestSetToolheadMappingSetsDefaultDisplayName(t *testing.T) {
	bridge := newTestBridge(t)
	printerID := "printer_snapmaker"
	saveMoonrakerPrinter(t, bridge, printerID, "Snapmaker U1", 2)

	if err := bridge.SetToolheadMapping(printerID, 1, 99); err != nil {
		t.Fatalf("SetToolheadMapping failed: %v", err)
	}

	var displayName string
	err := bridge.DB.QueryRow(
		"SELECT display_name FROM printer_slots WHERE slot_id = ?",
		ToolheadSlotID(printerID, 1),
	).Scan(&displayName)
	if err != nil {
		t.Fatalf("query display_name failed: %v", err)
	}
	if displayName != "Toolhead 2" {
		t.Fatalf("expected default display_name Toolhead 2, got %q", displayName)
	}
}
