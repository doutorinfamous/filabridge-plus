package spoolman

import "testing"

func TestIsValidTrayUUID(t *testing.T) {
	tests := []struct {
		uuid string
		want bool
	}{
		{"", false},
		{"unknown", false},
		{"0000000000000000", false},
		{"abc123def456", true},
	}
	for _, tt := range tests {
		if got := IsValidTrayUUID(tt.uuid); got != tt.want {
			t.Errorf("IsValidTrayUUID(%q) = %v, want %v", tt.uuid, got, tt.want)
		}
	}
}

func TestLengthToWeight(t *testing.T) {
	w := LengthToWeight(100, "PLA")
	if w <= 0 {
		t.Errorf("expected positive weight, got %f", w)
	}
}

func TestActiveTrayMatchesUniqueIDVariants(t *testing.T) {
	stored := `"A1_03919C461204338_AMS_03C12A3C0425658_tray_3"`
	parsed := "A1_03919C461204338_AMS_03C12A3C0425658_tray_3"
	if !activeTrayMatches(stored, parsed, "sensor.bambu_lab_a1_ams_tray_3", "A1_03919C461204338_AMS_03C12A3C0425658_tray_3") {
		t.Fatal("expected match for entity_id and unique_id")
	}
	if !activeTrayMatches(stored, parsed, "sensor.A1_03919C461204338_AMS_03C12A3C0425658_tray_3") {
		t.Fatal("expected match for sensor.{unique_id} shorthand")
	}
}
