package snapmaker

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"filabridge/spoolman"
)

func TestSpoolToSnapmakerFilamentConfig(t *testing.T) {
	spool := spoolman.Spool{
		ID:       1,
		Material: "petg",
		Brand:    "Polymaker",
		Filament: &spoolman.Filament{
			Name:     "PolyLite Matte",
			Material: "PETG",
			ColorHex: "#ff5733",
			Vendor:   &spoolman.Vendor{Name: "Polymaker"},
		},
	}

	cfg := SpoolToSnapmakerFilamentConfig(spool)
	if cfg.Vendor != "Polymaker" {
		t.Fatalf("expected vendor Polymaker, got %q", cfg.Vendor)
	}
	if cfg.Type != "PETG" {
		t.Fatalf("expected type PETG, got %q", cfg.Type)
	}
	if cfg.SubType != "Matte" {
		t.Fatalf("expected sub type Matte, got %q", cfg.SubType)
	}
	if cfg.ColorRGBA != "FF5733FF" {
		t.Fatalf("expected color FF5733FF, got %q", cfg.ColorRGBA)
	}
}

func TestBuildSetPrintFilamentConfigGcode(t *testing.T) {
	got := BuildSetPrintFilamentConfigGcode(2, SnapmakerFilamentConfig{
		Vendor:    "generic",
		Type:      "PLA",
		SubType:   "generic",
		ColorRGBA: "AABBCCDD",
	})

	want := "SET_PRINT_FILAMENT_CONFIG CONFIG_EXTRUDER=2 VENDOR=generic FILAMENT_TYPE=PLA FILAMENT_SUBTYPE=generic FILAMENT_COLOR_RGBA=AABBCCDD FORCE=1"
	if got != want {
		t.Fatalf("unexpected gcode:\n got:  %q\n want: %q", got, want)
	}
}

func TestBuildClearFilamentConfigGcode(t *testing.T) {
	got := BuildClearFilamentConfigGcode(1)
	if !strings.Contains(got, "CONFIG_EXTRUDER=1") {
		t.Fatalf("expected extruder 1 in gcode, got %q", got)
	}
	if !strings.Contains(got, "VENDOR=NONE") {
		t.Fatalf("expected NONE vendor in gcode, got %q", got)
	}
}

func TestRunGcodeScript(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/printer/gcode/script" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}

		var req moonrakerGcodeScriptRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if !strings.Contains(req.Script, "SET_PRINT_FILAMENT_CONFIG") {
			t.Fatalf("unexpected script: %q", req.Script)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{"result": "ok"})
	}))
	defer server.Close()

	client := NewMoonrakerClient(server.URL, "", 5, 60)
	if err := client.RunGcodeScript(BuildSetPrintFilamentConfigGcode(0, SnapmakerFilamentConfig{
		Vendor:    "generic",
		Type:      "PLA",
		SubType:   "generic",
		ColorRGBA: "FFFFFFFF",
	})); err != nil {
		t.Fatalf("RunGcodeScript failed: %v", err)
	}
}

func TestColorHexToRGBA(t *testing.T) {
	tests := map[string]string{
		"#AABBCC":   "AABBCCFF",
		"AABBCCDD":  "AABBCCDD",
		"invalid":   defaultFilamentColorRGBA,
		"":          defaultFilamentColorRGBA,
	}

	for input, want := range tests {
		if got := colorHexToRGBA(input); got != want {
			t.Fatalf("colorHexToRGBA(%q) = %q, want %q", input, got, want)
		}
	}
}
