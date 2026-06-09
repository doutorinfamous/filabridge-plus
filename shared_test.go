package main

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
