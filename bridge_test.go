package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShouldProcessCompletedJob(t *testing.T) {
	tests := []struct {
		name             string
		rawState         string
		filename         string
		printDuration    float64
		alreadyProcessed bool
		want             bool
	}{
		{
			name:             "complete unprocessed job",
			rawState:         "complete",
			filename:         "jobs/test.gcode",
			printDuration:    120,
			alreadyProcessed: false,
			want:             true,
		},
		{
			name:             "complete already processed",
			rawState:         "complete",
			filename:         "jobs/test.gcode",
			printDuration:    120,
			alreadyProcessed: true,
			want:             false,
		},
		{
			name:             "standby should not trigger alternate path",
			rawState:         "standby",
			filename:         "jobs/test.gcode",
			printDuration:    120,
			alreadyProcessed: false,
			want:             false,
		},
		{
			name:             "complete without filename",
			rawState:         "complete",
			filename:         "",
			printDuration:    120,
			alreadyProcessed: false,
			want:             false,
		},
		{
			name:             "complete without duration",
			rawState:         "complete",
			filename:         "jobs/test.gcode",
			printDuration:    0,
			alreadyProcessed: false,
			want:             false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldProcessCompletedJob(tt.rawState, tt.filename, tt.printDuration, tt.alreadyProcessed)
			if got != tt.want {
				t.Fatalf("shouldProcessCompletedJob() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilamentLengthMmToGrams(t *testing.T) {
	// 1000mm of 1.75mm PLA (~1.24 g/cm³) ≈ 2.98g
	weight := filamentLengthMmToGrams(1000, 1.75, 1.24)
	if weight <= 0 {
		t.Fatal("expected positive weight")
	}
	if weight < 2.5 || weight > 3.5 {
		t.Fatalf("unexpected weight %.3fg", weight)
	}
}

func TestProcessedJobsDedup(t *testing.T) {
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

	printerID := "printer_test"
	filename := "jobs/example.gcode"

	processed, err := bridge.IsJobProcessed(printerID, filename)
	if err != nil {
		t.Fatalf("IsJobProcessed failed: %v", err)
	}
	if processed {
		t.Fatal("job should not be processed initially")
	}

	if err := bridge.MarkJobProcessed(printerID, filename); err != nil {
		t.Fatalf("MarkJobProcessed failed: %v", err)
	}

	processed, err = bridge.IsJobProcessed(printerID, filename)
	if err != nil {
		t.Fatalf("IsJobProcessed failed: %v", err)
	}
	if !processed {
		t.Fatal("job should be marked processed")
	}

	if err := bridge.ClearProcessedJob(printerID, filename); err != nil {
		t.Fatalf("ClearProcessedJob failed: %v", err)
	}

	processed, err = bridge.IsJobProcessed(printerID, filename)
	if err != nil {
		t.Fatalf("IsJobProcessed failed: %v", err)
	}
	if processed {
		t.Fatal("job should be cleared after reprint start")
	}
}

func TestProcessFilamentUsageUnmappedSpoolAddsError(t *testing.T) {
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

	err = bridge.processFilamentUsage("TestPrinter", map[int]float64{0: 12.5}, "test.gcode")
	if err == nil {
		t.Fatal("expected error when no spool is mapped")
	}

	errors := bridge.GetPrintErrors()
	if len(errors) == 0 {
		t.Fatal("expected print error for unmapped spool")
	}
}

func TestProcessFilamentUsageDoesNotRemapAcrossMappedToolheads(t *testing.T) {
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

	printerName := "Snapmaker U1"
	if err := bridge.SetToolheadMapping(printerName, 1, 42); err != nil {
		t.Fatalf("failed to map toolhead 1: %v", err)
	}

	err = bridge.processFilamentUsage(printerName, map[int]float64{0: 9.15}, "test.gcode")
	if err == nil {
		t.Fatal("expected error when G-code targets extruder 0 but only toolhead 1 is mapped")
	}

	errors := bridge.GetPrintErrors()
	foundExtruder0Error := false
	for _, pe := range errors {
		if strings.Contains(pe.Error, "extruder 0") && strings.Contains(pe.Error, "Toolhead 0") {
			foundExtruder0Error = true
		}
	}
	if !foundExtruder0Error {
		t.Fatalf("expected explicit extruder 0 error, got %v", errors)
	}
}
