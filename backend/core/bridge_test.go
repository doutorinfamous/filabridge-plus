package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestBridge(t *testing.T) *FilamentBridge {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	bridge, err := NewFilamentBridge(&Config{DBFile: dbPath})
	if err != nil {
		t.Fatalf("failed to create bridge: %v", err)
	}
	t.Cleanup(func() {
		bridge.Close()
		os.RemoveAll(dir)
	})
	return bridge
}

func TestPrintJobsDedup(t *testing.T) {
	bridge := newTestBridge(t)

	printerID := "printer_test"
	filename := "jobs/example.gcode"

	processed, err := bridge.IsJobProcessed(printerID, filename)
	if err != nil {
		t.Fatalf("IsJobProcessed failed: %v", err)
	}
	if processed {
		t.Fatal("job should not be processed initially")
	}

	jobID, err := bridge.StartPrintJob(printerID, filename)
	if err != nil {
		t.Fatalf("StartPrintJob failed: %v", err)
	}
	if jobID <= 0 {
		t.Fatal("expected a valid job ID")
	}

	// Starting the same job again must reuse the open job (idempotent polling)
	sameJobID, err := bridge.StartPrintJob(printerID, filename)
	if err != nil {
		t.Fatalf("StartPrintJob (repeat) failed: %v", err)
	}
	if sameJobID != jobID {
		t.Fatalf("expected same open job %d, got %d", jobID, sameJobID)
	}

	processed, err = bridge.IsJobProcessed(printerID, filename)
	if err != nil {
		t.Fatalf("IsJobProcessed failed: %v", err)
	}
	if processed {
		t.Fatal("open job should not count as processed")
	}

	if err := bridge.FinishPrintJob(printerID, filename, JobStatusCompleted); err != nil {
		t.Fatalf("FinishPrintJob failed: %v", err)
	}

	processed, err = bridge.IsJobProcessed(printerID, filename)
	if err != nil {
		t.Fatalf("IsJobProcessed failed: %v", err)
	}
	if !processed {
		t.Fatal("job should be marked processed after finish")
	}

	// Reprint of the same file opens a new job and stops counting as processed
	newJobID, err := bridge.StartPrintJob(printerID, filename)
	if err != nil {
		t.Fatalf("StartPrintJob (reprint) failed: %v", err)
	}
	if newJobID == jobID {
		t.Fatal("reprint should create a new job")
	}

	processed, err = bridge.IsJobProcessed(printerID, filename)
	if err != nil {
		t.Fatalf("IsJobProcessed failed: %v", err)
	}
	if processed {
		t.Fatal("job should not be processed after reprint start")
	}
}

func TestProcessFilamentUsageUnmappedSpoolAddsError(t *testing.T) {
	bridge := newTestBridge(t)

	err := bridge.ProcessFilamentUsage("TestPrinter", map[int]float64{0: 12.5}, "test.gcode")
	if err == nil {
		t.Fatal("expected error when no spool is mapped")
	}

	errors := bridge.GetPrintErrors()
	if len(errors) == 0 {
		t.Fatal("expected print error for unmapped spool")
	}
	if errors[0].Grams != 12.5 {
		t.Fatalf("expected 12.5g on error, got %v", errors[0].Grams)
	}
	if errors[0].ToolheadID == nil || *errors[0].ToolheadID != 0 {
		t.Fatalf("expected toolhead 0 on error, got %v", errors[0].ToolheadID)
	}
}

func TestProcessFilamentUsageDoesNotRemapAcrossMappedToolheads(t *testing.T) {
	bridge := newTestBridge(t)

	printerName := "Snapmaker U1"
	if err := bridge.SetToolheadMapping(printerName, 1, 42); err != nil {
		t.Fatalf("failed to map toolhead 1: %v", err)
	}

	err := bridge.ProcessFilamentUsage(printerName, map[int]float64{0: 9.15}, "test.gcode")
	if err == nil {
		t.Fatal("expected error when G-code targets extruder 0 but only toolhead 1 is mapped")
	}

	errors := bridge.GetPrintErrors()
	foundExtruder0Error := false
	for _, pe := range errors {
		if strings.Contains(pe.Error, "extruder 0") && strings.Contains(pe.Error, "Toolhead 1") {
			foundExtruder0Error = true
		}
	}
	if !foundExtruder0Error {
		t.Fatalf("expected explicit extruder 0 error, got %v", errors)
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
