package main

import (
	"path/filepath"
	"testing"
)

func TestParseLocationParamToolheadOneBased(t *testing.T) {
	dir := t.TempDir()
	bridge, err := NewFilamentBridge(&Config{
		DBFile:      filepath.Join(dir, "test.db"),
		SpoolmanURL: "http://127.0.0.1:1",
	})
	if err != nil {
		t.Fatalf("failed to create bridge: %v", err)
	}
	t.Cleanup(func() { bridge.Close() })

	if err := bridge.SavePrinterConfig("printer1", PrinterConfig{
		Name:      "My Printer",
		IPAddress: "127.0.0.1",
		Toolheads: 2,
	}); err != nil {
		t.Fatalf("failed to save printer config: %v", err)
	}

	printerName, toolheadID, locationName, isPrinterLocation, err := bridge.parseLocationParam("My Printer - Toolhead 1")
	if err != nil {
		t.Fatalf("parseLocationParam failed: %v", err)
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

	_, toolheadID2, _, isPrinterLocation2, err := bridge.parseLocationParam("My Printer - Toolhead 2")
	if err != nil {
		t.Fatalf("parseLocationParam failed: %v", err)
	}
	if !isPrinterLocation2 || toolheadID2 != 1 {
		t.Fatalf("expected toolhead_id 1 for Toolhead 2, got %d (printer=%v)", toolheadID2, isPrinterLocation2)
	}
}

func TestDefaultToolheadDisplayName(t *testing.T) {
	if got := DefaultToolheadDisplayName(0); got != "Toolhead 1" {
		t.Fatalf("expected Toolhead 1, got %q", got)
	}
	if got := DefaultToolheadDisplayName(3); got != "Toolhead 4" {
		t.Fatalf("expected Toolhead 4, got %q", got)
	}
}
