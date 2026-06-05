package main

import (
	"encoding/json"
	"testing"
)

func TestNormalizeMoonrakerState(t *testing.T) {
	tests := map[string]string{
		"printing": StatePrinting,
		"paused":   StatePrinting,
		"complete": StateFinished,
		"standby":  StateIdle,
		"error":    StateError,
	}

	for input, expected := range tests {
		if got := normalizeMoonrakerState(input); got != expected {
			t.Fatalf("normalizeMoonrakerState(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestDetectPrinterModelSnapmakerU1(t *testing.T) {
	if got := detectPrinterModel("snapmaker-u1"); got != ModelSnapmakerU1 {
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
					"print_duration": 120.5
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
	if result.Status.VirtualSDCard.Progress != 0.42 {
		t.Fatalf("unexpected progress: %v", result.Status.VirtualSDCard.Progress)
	}
}

func TestIsMoonrakerPrintingState(t *testing.T) {
	if !isMoonrakerPrintingState("printing") {
		t.Fatal("printing should be active")
	}
	if !isMoonrakerPrintingState("paused") {
		t.Fatal("paused should be active")
	}
	if isMoonrakerFinishedState("printing") {
		t.Fatal("printing should not be finished")
	}
}

func TestEscapeMoonrakerFilePath(t *testing.T) {
	got := escapeMoonrakerFilePath("jobs/test print.gcode")
	want := "jobs/test%20print.gcode"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
