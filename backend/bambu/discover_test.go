package bambu

import (
	"testing"

	"filabridge/core"
	"filabridge/homeassistant"
)

func TestNormalizeBambuState(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"running", core.StatePrinting},
		{"Running", core.StatePrinting},
		{"pause", core.StatePrinting},
		{"idle", core.StateIdle},
		{"finish", core.StateIdle},
		{"unavailable", core.StateOffline},
		{"", core.StateOffline},
	}
	for _, tt := range tests {
		if got := normalizeBambuState(tt.in); got != tt.want {
			t.Errorf("normalizeBambuState(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseHAProgressPercent(t *testing.T) {
	if got := parseHAProgressPercent("45"); got != 0.45 {
		t.Fatalf("expected 0.45, got %v", got)
	}
	if got := parseHAProgressPercent("0.45"); got != 0.45 {
		t.Fatalf("expected 0.45, got %v", got)
	}
}

func TestResolveBambuJobNameUsesTaskNameSensorState(t *testing.T) {
	stateMap := map[string]homeassistant.State{
		"sensor.bambu_lab_a1_task_name": {EntityID: "sensor.bambu_lab_a1_task_name", State: "Cable clip"},
		"sensor.bambu_lab_a1_gcode_file": {
			EntityID: "sensor.bambu_lab_a1_gcode_file",
			State:    "plate_1.gcode",
		},
	}

	got := resolveBambuJobName(stateMap, "sensor.bambu_lab_a1_task_name", "sensor.bambu_lab_a1_gcode_file")
	if got != "Cable clip" {
		t.Fatalf("expected task name sensor state, got %q", got)
	}
}

func TestResolveBambuJobNameFallsBackToGcodeFile(t *testing.T) {
	stateMap := map[string]homeassistant.State{
		"sensor.bambu_lab_a1_task_name":  {EntityID: "sensor.bambu_lab_a1_task_name", State: "unknown"},
		"sensor.bambu_lab_a1_gcode_file": {EntityID: "sensor.bambu_lab_a1_gcode_file", State: "plate_1.gcode"},
	}

	got := resolveBambuJobName(stateMap, "sensor.bambu_lab_a1_task_name", "sensor.bambu_lab_a1_gcode_file")
	if got != "plate_1.gcode" {
		t.Fatalf("expected gcode fallback, got %q", got)
	}
}
