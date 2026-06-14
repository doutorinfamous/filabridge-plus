package snapmaker

import (
	"encoding/json"
	"testing"

	"filabridge/core"
)

func TestNormalizeMoonrakerState(t *testing.T) {
	tests := map[string]string{
		"printing":  core.StatePrinting,
		"paused":    core.StatePrinting,
		"complete":  core.StateFinished,
		"cancelled": core.StateFinished,
		"standby":   core.StateIdle,
		"error":     core.StateError,
	}

	for input, expected := range tests {
		if got := NormalizeMoonrakerState(input); got != expected {
			t.Fatalf("NormalizeMoonrakerState(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestDetectPrinterModelSnapmakerU1(t *testing.T) {
	if got := DetectPrinterModel("snapmaker-u1"); got != ModelSnapmakerU1 {
		t.Fatalf("expected %s, got %s", ModelSnapmakerU1, got)
	}
}

func TestParseMoonrakerObjectsQuery(t *testing.T) {
	payload := []byte(`{
		"result": {
			"status": {
				"print_stats": {
					"state": "printing",
					"filename": "jobs/example.gcode",
					"print_duration": 120.5,
					"filament_used": 4567.8
				},
				"virtual_sdcard": {
					"progress": 0.42
				}
			}
		}
	}`)

	var envelope moonrakerResponse
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("failed to decode envelope: %v", err)
	}

	var result moonrakerObjectsQueryResult
	if err := json.Unmarshal(envelope.Result, &result); err != nil {
		t.Fatalf("failed to decode result: %v", err)
	}

	if result.Status.PrintStats.State != "printing" {
		t.Fatalf("expected printing state, got %s", result.Status.PrintStats.State)
	}
	if result.Status.PrintStats.Filename != "jobs/example.gcode" {
		t.Fatalf("unexpected filename: %s", result.Status.PrintStats.Filename)
	}
	if result.Status.PrintStats.PrintDuration != 120.5 {
		t.Fatalf("unexpected print duration: %v", result.Status.PrintStats.PrintDuration)
	}
	if result.Status.PrintStats.FilamentUsed != 4567.8 {
		t.Fatalf("unexpected filament used: %v", result.Status.PrintStats.FilamentUsed)
	}
	if result.Status.VirtualSDCard.Progress != 0.42 {
		t.Fatalf("unexpected progress: %v", result.Status.VirtualSDCard.Progress)
	}
}

func TestIsMoonrakerPrintingState(t *testing.T) {
	if !IsMoonrakerPrintingState("printing") {
		t.Fatal("printing should be active")
	}
	if !IsMoonrakerPrintingState("paused") {
		t.Fatal("paused should be active")
	}
	if IsMoonrakerFinishedState("printing") {
		t.Fatal("printing should not be finished")
	}
}

func TestIsMoonrakerCancelledState(t *testing.T) {
	if !IsMoonrakerCancelledState("cancelled") {
		t.Fatal("cancelled should be detected")
	}
	if IsMoonrakerCancelledState("complete") {
		t.Fatal("complete should not be cancelled")
	}
}

func TestComputeTimeRemainingSeconds(t *testing.T) {
	t.Run("slicer estimate", func(t *testing.T) {
		got := ComputeTimeRemainingSeconds(600, 0.5, 3600)
		if got == nil || *got != 3000 {
			t.Fatalf("expected 3000, got %v", got)
		}
	})

	t.Run("slicer estimate clamps negative", func(t *testing.T) {
		got := ComputeTimeRemainingSeconds(4000, 0.9, 3600)
		if got == nil || *got != 0 {
			t.Fatalf("expected 0, got %v", got)
		}
	})

	t.Run("progress fallback", func(t *testing.T) {
		got := ComputeTimeRemainingSeconds(120, 0.4, 0)
		if got == nil || *got != 180 {
			t.Fatalf("expected 180, got %v", got)
		}
	})

	t.Run("unknown when no data", func(t *testing.T) {
		if got := ComputeTimeRemainingSeconds(0, 0, 0); got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})
}

func TestFormatDurationSeconds(t *testing.T) {
	tests := map[float64]string{
		0:    "0s",
		45:   "45s",
		90:   "1m 30s",
		3661: "1h 1m",
		-1:   "--",
	}

	for input, expected := range tests {
		if got := FormatDurationSeconds(input); got != expected {
			t.Fatalf("FormatDurationSeconds(%v) = %q, want %q", input, got, expected)
		}
	}
}

func TestEscapeMoonrakerFilePath(t *testing.T) {
	got := escapeMoonrakerFilePath("jobs/test print.gcode")
	want := "jobs/test%20print.gcode"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestParsePrintTaskConfigReprintExtruderMapTable(t *testing.T) {
	payload := []byte(`{
		"result": {
			"status": {
				"print_task_config": {
					"extruder_map_table": [0, 1, 2, 3, 0, 0],
					"extruders_used": [false, false, false, false],
					"reprint_info": {
						"extruder_map_table": [0, 1, 3, 3, 0, 2],
						"extruders_used": [true, true, true, true]
					}
				}
			}
		}
	}`)

	var envelope moonrakerResponse
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("failed to decode envelope: %v", err)
	}

	var result moonrakerObjectsQueryPrintTaskResult
	if err := json.Unmarshal(envelope.Result, &result); err != nil {
		t.Fatalf("failed to decode result: %v", err)
	}

	cfg := result.Status.PrintTaskConfig
	if len(cfg.ReprintInfo.ExtruderMapTable) != 6 {
		t.Fatalf("expected 6 reprint map entries, got %d", len(cfg.ReprintInfo.ExtruderMapTable))
	}
	if cfg.ReprintInfo.ExtruderMapTable[5] != 2 {
		t.Fatalf("expected reprint map index 5 -> 2, got %d", cfg.ReprintInfo.ExtruderMapTable[5])
	}
	if !cfg.ReprintInfo.ExtrudersUsed[0] || !cfg.ReprintInfo.ExtrudersUsed[3] {
		t.Fatalf("expected all reprint extruders used, got %v", cfg.ReprintInfo.ExtrudersUsed)
	}
}

func TestFilamentLengthMmToGrams(t *testing.T) {
	// 1000mm of 1.75mm PLA (~1.24 g/cm³) ≈ 2.98g
	weight := FilamentLengthMmToGrams(1000, 1.75, 1.24)
	if weight <= 0 {
		t.Fatal("expected positive weight")
	}
	if weight < 2.5 || weight > 3.5 {
		t.Fatalf("unexpected weight %.3fg", weight)
	}
}
