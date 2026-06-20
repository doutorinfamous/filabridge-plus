package snapmaker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filabridge/core"
	"filabridge/spoolman"
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

func TestShouldProcessCancelledJob(t *testing.T) {
	tests := []struct {
		name             string
		rawState         string
		filename         string
		printDuration    float64
		alreadyProcessed bool
		want             bool
	}{
		{
			name:             "cancelled unprocessed job",
			rawState:         "cancelled",
			filename:         "jobs/test.gcode",
			printDuration:    120,
			alreadyProcessed: false,
			want:             true,
		},
		{
			name:             "cancelled already processed",
			rawState:         "cancelled",
			filename:         "jobs/test.gcode",
			printDuration:    120,
			alreadyProcessed: true,
			want:             false,
		},
		{
			name:             "complete should not trigger cancelled path",
			rawState:         "complete",
			filename:         "jobs/test.gcode",
			printDuration:    120,
			alreadyProcessed: false,
			want:             false,
		},
		{
			name:             "cancelled without filename",
			rawState:         "cancelled",
			filename:         "",
			printDuration:    120,
			alreadyProcessed: false,
			want:             false,
		},
		{
			name:             "cancelled without duration",
			rawState:         "cancelled",
			filename:         "jobs/test.gcode",
			printDuration:    0,
			alreadyProcessed: false,
			want:             false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldProcessCancelledJob(tt.rawState, tt.filename, tt.printDuration, tt.alreadyProcessed)
			if got != tt.want {
				t.Fatalf("shouldProcessCancelledJob() = %v, want %v", got, tt.want)
			}
		})
	}
}

func newTestBridge(t *testing.T) *core.FilamentBridge {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	bridge, err := core.NewFilamentBridge(&core.Config{DBFile: dbPath})
	if err != nil {
		t.Fatalf("failed to create bridge: %v", err)
	}
	t.Cleanup(func() {
		bridge.Close()
		os.RemoveAll(dir)
	})
	return bridge
}

func newMoonrakerStub(t *testing.T, gcodeContent string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/server/files/gcodes/"):
			w.Write([]byte(gcodeContent))
		case strings.Contains(r.URL.Path, "/server/files/metadata"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"estimated_time": 3600.0,
				},
			})
		case strings.Contains(r.URL.RawQuery, "print_task_config"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"status": map[string]any{
						"print_task_config": map[string]any{},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestResolvePartialFilamentUsagePrintStats(t *testing.T) {
	server := newMoonrakerStub(t, "; total filament used [g] = 10.00\n")
	bridge := newTestBridge(t)

	client := NewMoonrakerClient(server.URL, "", 10, 10)
	status := &PrinterStatus{FilamentUsed: 5000}

	resolution, err := resolvePartialFilamentUsage(bridge, client, "TestPrinter", "jobs/test.gcode", status, 10)
	if err != nil {
		t.Fatalf("resolvePartialFilamentUsage failed: %v", err)
	}
	if resolution.Source != "print_stats" {
		t.Fatalf("expected print_stats source, got %s", resolution.Source)
	}
	if resolution.Usage[0] <= 0 {
		t.Fatalf("expected positive usage from print_stats, got %v", resolution.Usage)
	}
}

func TestResolvePartialFilamentUsageProgressFallback(t *testing.T) {
	server := newMoonrakerStub(t, "; total filament used [g] = 10.00\n")
	bridge := newTestBridge(t)

	client := NewMoonrakerClient(server.URL, "", 10, 10)
	status := &PrinterStatus{Progress: 0.4}

	resolution, err := resolvePartialFilamentUsage(bridge, client, "TestPrinter", "jobs/test.gcode", status, 10)
	if err != nil {
		t.Fatalf("resolvePartialFilamentUsage failed: %v", err)
	}
	if resolution.Source != "progress" {
		t.Fatalf("expected progress source, got %s", resolution.Source)
	}
	if resolution.Usage[0] != 4 {
		t.Fatalf("expected 4g at 40%% progress, got %v", resolution.Usage)
	}
}

func TestHandlePrintCancelledCreatesConfirmationWithoutDebiting(t *testing.T) {
	server := newMoonrakerStub(t, "; total filament used [g] = 10.00\n")
	bridge := newTestBridge(t)

	spoolmanCalled := false
	spoolServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch || r.Method == http.MethodPut {
			spoolmanCalled = true
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(spoolServer.Close)

	bridge.Spoolman = spoolman.NewClient(spoolServer.URL, 10, "", "")
	bridge.Config = &core.Config{
		PrinterTimeout:             10,
		PrinterFileDownloadTimeout: 10,
	}

	printerID := "printer1"
	if err := bridge.SavePrinterConfig(printerID, core.PrinterConfig{
		Name:      "Test Printer",
		IPAddress: server.URL,
		Toolheads: 1,
	}); err != nil {
		t.Fatalf("SavePrinterConfig failed: %v", err)
	}
	if err := bridge.SetToolheadMapping(printerID, 0, 42); err != nil {
		t.Fatalf("SetToolheadMapping failed: %v", err)
	}

	status := &PrinterStatus{Progress: 0.4}
	err := handlePrintCancelled(bridge, printerID, core.PrinterConfig{
		Name:      "Test Printer",
		IPAddress: server.URL,
	}, "jobs/test.gcode", status)
	if err != nil {
		t.Fatalf("handlePrintCancelled failed: %v", err)
	}
	if spoolmanCalled {
		t.Fatal("expected cancelled print not to debit Spoolman automatically")
	}

	errors := bridge.GetPrintErrors()
	if len(errors) != 1 {
		t.Fatalf("expected 1 confirmation, got %d", len(errors))
	}
	if errors[0].Kind != core.PrintErrorKindUsageConfirm {
		t.Fatalf("expected usage_confirmation, got %q", errors[0].Kind)
	}
}
