package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func newTestBridgeWithSpoolman(t *testing.T, handler http.HandlerFunc) *FilamentBridge {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	dir := t.TempDir()
	bridge, err := NewFilamentBridge(&Config{
		DBFile:      filepath.Join(dir, "test.db"),
		SpoolmanURL: server.URL,
	})
	if err != nil {
		t.Fatalf("failed to create bridge: %v", err)
	}
	t.Cleanup(func() { bridge.Close() })

	if err := bridge.SetConfigValue(ConfigKeySpoolmanURL, server.URL); err != nil {
		t.Fatalf("failed to set spoolman url: %v", err)
	}

	return bridge
}

func spoolmanUpdateStub(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/spool/42":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          42,
				"used_weight": 100.0,
				"remaining_weight": 900.0,
			})
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/spool/42":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 42})
		default:
			http.NotFound(w, r)
		}
	}
}

func addUnmappedPrintError(t *testing.T, bridge *FilamentBridge, printerID, jobName string, toolheadID int, grams float64) string {
	t.Helper()

	before := len(bridge.GetPrintErrors())
	bridge.AddPrintError(PrintErrorInput{
		PrinterID:   printerID,
		PrinterName: printerID,
		JobName:     jobName,
		Error:       "unmapped toolhead",
		ToolheadID:  toolheadID,
		Grams:       grams,
	})

	errors := bridge.GetPrintErrors()
	if len(errors) != before+1 {
		t.Fatalf("expected %d print errors, got %d", before+1, len(errors))
	}
	return errors[len(errors)-1].ID
}

func getJobStatus(t *testing.T, bridge *FilamentBridge, printerID, jobName string) string {
	t.Helper()

	var status string
	err := bridge.DB.QueryRow(
		"SELECT status FROM print_jobs WHERE printer_id = ? AND job_name = ? ORDER BY id DESC LIMIT 1",
		printerID, jobName,
	).Scan(&status)
	if err != nil {
		t.Fatalf("failed to query job status: %v", err)
	}
	return status
}

func TestFinishPrintJobUpdatesFailedStatus(t *testing.T) {
	bridge := newTestBridge(t)
	printerID := "printer1"
	jobName := "test.gcode"

	if _, err := bridge.StartPrintJob(printerID, jobName); err != nil {
		t.Fatalf("StartPrintJob failed: %v", err)
	}
	if err := bridge.FinishPrintJob(printerID, jobName, JobStatusFailed); err != nil {
		t.Fatalf("FinishPrintJob failed: %v", err)
	}
	if status := getJobStatus(t, bridge, printerID, jobName); status != JobStatusFailed {
		t.Fatalf("expected failed status, got %q", status)
	}

	if err := bridge.FinishPrintJob(printerID, jobName, JobStatusCompleted); err != nil {
		t.Fatalf("FinishPrintJob (completed) failed: %v", err)
	}
	if status := getJobStatus(t, bridge, printerID, jobName); status != JobStatusCompleted {
		t.Fatalf("expected completed status, got %q", status)
	}
}

func TestResolvePrintErrorAssignSpool(t *testing.T) {
	bridge := newTestBridgeWithSpoolman(t, spoolmanUpdateStub(t))

	printerID := "printer1"
	jobName := "Dress.gcode"
	if _, err := bridge.StartPrintJob(printerID, jobName); err != nil {
		t.Fatalf("StartPrintJob failed: %v", err)
	}
	if err := bridge.FinishPrintJob(printerID, jobName, JobStatusFailed); err != nil {
		t.Fatalf("FinishPrintJob failed: %v", err)
	}

	errorID := addUnmappedPrintError(t, bridge, printerID, jobName, 2, 12.57)

	if err := bridge.ResolvePrintError(errorID, ResolveActionAssignSpool, 42); err != nil {
		t.Fatalf("ResolvePrintError failed: %v", err)
	}

	if status := getJobStatus(t, bridge, printerID, jobName); status != JobStatusCompleted {
		t.Fatalf("expected completed status, got %q", status)
	}
	if bridge.GetPrintErrors() != nil && len(bridge.GetPrintErrors()) > 0 {
		t.Fatal("expected error to be acknowledged")
	}

	var grams float64
	err := bridge.DB.QueryRow(
		"SELECT grams FROM filament_usage WHERE printer_id = ? AND toolhead_id = ? AND spool_id = ?",
		printerID, 2, 42,
	).Scan(&grams)
	if err != nil {
		t.Fatalf("failed to query filament usage: %v", err)
	}
	if grams != 12.57 {
		t.Fatalf("expected 12.57g logged, got %v", grams)
	}
}

func TestResolvePrintErrorDismissAll(t *testing.T) {
	bridge := newTestBridge(t)

	printerID := "printer1"
	jobName := "Dress.gcode"
	if _, err := bridge.StartPrintJob(printerID, jobName); err != nil {
		t.Fatalf("StartPrintJob failed: %v", err)
	}
	if err := bridge.FinishPrintJob(printerID, jobName, JobStatusFailed); err != nil {
		t.Fatalf("FinishPrintJob failed: %v", err)
	}

	bridge.AddPrintError(PrintErrorInput{
		PrinterID: printerID, PrinterName: printerID, JobName: jobName,
		Error: "err1", ToolheadID: 0, Grams: 5,
	})
	bridge.AddPrintError(PrintErrorInput{
		PrinterID: printerID, PrinterName: printerID, JobName: jobName,
		Error: "err2", ToolheadID: 2, Grams: 12.57,
	})

	errors := bridge.GetPrintErrors()
	if len(errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(errors))
	}

	if err := bridge.ResolvePrintError(errors[0].ID, ResolveActionDismiss, 0); err != nil {
		t.Fatalf("ResolvePrintError dismiss failed: %v", err)
	}

	if status := getJobStatus(t, bridge, printerID, jobName); status != JobStatusCompleted {
		t.Fatalf("expected completed status, got %q", status)
	}
	if len(bridge.GetPrintErrors()) != 0 {
		t.Fatal("expected all errors dismissed")
	}

	var count int
	if err := bridge.DB.QueryRow(
		"SELECT COUNT(*) FROM filament_usage WHERE printer_id = ?",
		printerID,
	).Scan(&count); err != nil {
		t.Fatalf("failed to count usage rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no filament usage, got %d rows", count)
	}
}

func TestResolvePrintErrorAssignSpoolPartialCompletion(t *testing.T) {
	bridge := newTestBridgeWithSpoolman(t, spoolmanUpdateStub(t))

	printerID := "printer1"
	jobName := "multi.gcode"
	if _, err := bridge.StartPrintJob(printerID, jobName); err != nil {
		t.Fatalf("StartPrintJob failed: %v", err)
	}
	if err := bridge.FinishPrintJob(printerID, jobName, JobStatusFailed); err != nil {
		t.Fatalf("FinishPrintJob failed: %v", err)
	}

	bridge.AddPrintError(PrintErrorInput{
		PrinterID: printerID, PrinterName: printerID, JobName: jobName,
		Error: "err1", ToolheadID: 0, Grams: 5,
	})
	errorID2 := addUnmappedPrintError(t, bridge, printerID, jobName, 2, 12.57)

	errors := bridge.GetPrintErrors()
	if len(errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(errors))
	}

	if err := bridge.ResolvePrintError(errorID2, ResolveActionAssignSpool, 42); err != nil {
		t.Fatalf("ResolvePrintError failed: %v", err)
	}

	if status := getJobStatus(t, bridge, printerID, jobName); status != JobStatusFailed {
		t.Fatalf("expected job to remain failed until all errors resolved, got %q", status)
	}
	if len(bridge.GetPrintErrors()) != 1 {
		t.Fatalf("expected 1 pending error, got %d", len(bridge.GetPrintErrors()))
	}
}
