package nfc

import (
	"path/filepath"
	"testing"

	"filabridge/core"
)

func TestParseLocationParamToolheadOneBased(t *testing.T) {
	dir := t.TempDir()
	bridge, err := core.NewFilamentBridge(&core.Config{
		DBFile:      filepath.Join(dir, "test.db"),
		SpoolmanURL: "http://127.0.0.1:1",
	})
	if err != nil {
		t.Fatalf("failed to create bridge: %v", err)
	}
	t.Cleanup(func() { bridge.Close() })

	if err := bridge.SavePrinterConfig("printer1", core.PrinterConfig{
		Name:      "My Printer",
		IPAddress: "127.0.0.1",
		Toolheads: 2,
	}); err != nil {
		t.Fatalf("failed to save printer config: %v", err)
	}

	printerName, toolheadID, locationName, isPrinterLocation, err := ParseLocationParam(bridge, "My Printer - Toolhead 1")
	if err != nil {
		t.Fatalf("ParseLocationParam failed: %v", err)
	}
	if !isPrinterLocation {
		t.Fatal("expected printer location")
	}
	if printerName != "My Printer" {
		t.Fatalf("expected printer name My Printer, got %q", printerName)
	}
	if toolheadID != 0 {
		t.Fatalf("expected toolhead_id 0 for Toolhead 1, got %d", toolheadID)
	}
	if locationName != "My Printer - Toolhead 1" {
		t.Fatalf("unexpected location name %q", locationName)
	}

	_, toolheadID2, _, isPrinterLocation2, err := ParseLocationParam(bridge, "My Printer - Toolhead 2")
	if err != nil {
		t.Fatalf("ParseLocationParam failed: %v", err)
	}
	if !isPrinterLocation2 || toolheadID2 != 1 {
		t.Fatalf("expected toolhead_id 1 for Toolhead 2, got %d (printer=%v)", toolheadID2, isPrinterLocation2)
	}
}

func newSessionTestBridge(t *testing.T) *core.FilamentBridge {
	t.Helper()

	dir := t.TempDir()
	bridge, err := core.NewFilamentBridge(&core.Config{
		DBFile:      filepath.Join(dir, "test.db"),
		SpoolmanURL: "http://127.0.0.1:1",
	})
	if err != nil {
		t.Fatalf("failed to create bridge: %v", err)
	}
	t.Cleanup(func() { bridge.Close() })

	if err := bridge.SavePrinterConfig("printer1", core.PrinterConfig{
		Name:      "My Printer",
		IPAddress: "127.0.0.1",
		Toolheads: 2,
	}); err != nil {
		t.Fatalf("failed to save printer config: %v", err)
	}
	return bridge
}

func TestCreateOrUpdateSessionBambuLocationOnly(t *testing.T) {
	bridge := newSessionTestBridge(t)
	const bambuLocation = "Bambu Lab A1 - AMS 1 Slot 4"

	session, err := CreateOrUpdateSession(bridge, "test-session", 0, "", 0, bambuLocation, true)
	if err != nil {
		t.Fatalf("CreateOrUpdateSession failed: %v", err)
	}
	if !session.HasLocation {
		t.Fatal("expected HasLocation true for Bambu AMS slot")
	}
	if session.HasSpool {
		t.Fatal("expected HasSpool false")
	}
	if session.LocationName != bambuLocation {
		t.Fatalf("expected location %q, got %q", bambuLocation, session.LocationName)
	}
}

func TestCreateOrUpdateSessionSpoolThenBambuLocation(t *testing.T) {
	bridge := newSessionTestBridge(t)
	const sessionID = "test-session"
	const bambuLocation = "Bambu Lab A1 - AMS 1 Slot 4"

	spoolSession, err := CreateOrUpdateSession(bridge, sessionID, 1, "", 0, "", false)
	if err != nil {
		t.Fatalf("CreateOrUpdateSession spool failed: %v", err)
	}
	if !spoolSession.HasSpool || spoolSession.HasLocation {
		t.Fatalf("expected spool only, got HasSpool=%v HasLocation=%v", spoolSession.HasSpool, spoolSession.HasLocation)
	}

	complete, err := CreateOrUpdateSession(bridge, sessionID, 0, "", 0, bambuLocation, true)
	if err != nil {
		t.Fatalf("CreateOrUpdateSession location failed: %v", err)
	}
	if !complete.HasSpool || !complete.HasLocation {
		t.Fatalf("expected complete session, got HasSpool=%v HasLocation=%v", complete.HasSpool, complete.HasLocation)
	}
	if !complete.IsComplete() {
		t.Fatal("expected IsComplete true")
	}
}

func TestCreateOrUpdateSessionMoonrakerToolheadStillWorks(t *testing.T) {
	bridge := newSessionTestBridge(t)
	const sessionID = "test-session"

	printerName, toolheadID, locationName, isPrinterLocation, err := ParseLocationParam(bridge, "My Printer - Toolhead 1")
	if err != nil {
		t.Fatalf("ParseLocationParam failed: %v", err)
	}

	session, err := CreateOrUpdateSession(bridge, sessionID, 0, printerName, toolheadID, locationName, isPrinterLocation)
	if err != nil {
		t.Fatalf("CreateOrUpdateSession failed: %v", err)
	}
	if !session.HasLocation {
		t.Fatal("expected HasLocation true for Moonraker toolhead")
	}
	if session.PrinterName != "My Printer" || session.ToolheadID != 0 {
		t.Fatalf("unexpected printer mapping: %q toolhead %d", session.PrinterName, session.ToolheadID)
	}
}

func TestGetSessionBambuLocationFlags(t *testing.T) {
	bridge := newSessionTestBridge(t)
	const sessionID = "test-session"
	const bambuLocation = "Bambu Lab A1 - AMS 1 Slot 4"

	if _, err := CreateOrUpdateSession(bridge, sessionID, 1, "", 0, bambuLocation, true); err != nil {
		t.Fatalf("CreateOrUpdateSession failed: %v", err)
	}

	session, err := GetSession(bridge, sessionID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if !session.HasLocation || !session.IsComplete() {
		t.Fatalf("expected complete session from GetSession, HasLocation=%v IsComplete=%v", session.HasLocation, session.IsComplete())
	}
}
