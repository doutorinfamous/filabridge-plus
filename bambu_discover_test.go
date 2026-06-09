package main

import "testing"

func TestNormalizeBambuState(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"running", StatePrinting},
		{"Running", StatePrinting},
		{"pause", StatePrinting},
		{"idle", StateIdle},
		{"finish", StateIdle},
		{"unavailable", StateOffline},
		{"", StateOffline},
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
