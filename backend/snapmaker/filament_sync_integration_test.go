package snapmaker

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"filabridge/core"
)

func TestTrySyncFilamentToToolheadDisabled(t *testing.T) {
	bridge, err := core.NewFilamentBridge(nil)
	if err != nil {
		t.Fatalf("NewFilamentBridge failed: %v", err)
	}
	defer bridge.DB.Close()

	if err := bridge.SetSyncFilamentToPrinterEnabled(false); err != nil {
		t.Fatalf("SetSyncFilamentToPrinterEnabled failed: %v", err)
	}

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := bridge.SavePrinterConfig("snap1", core.PrinterConfig{
		Name:      "Snapmaker",
		IPAddress: server.URL,
		Toolheads: 4,
	}); err != nil {
		t.Fatalf("SavePrinterConfig failed: %v", err)
	}

	bridge.Config = &core.Config{
		Printers: map[string]core.PrinterConfig{
			"snap1": {
				Name:      "Snapmaker",
				IPAddress: server.URL,
				Toolheads: 4,
			},
		},
	}

	if err := TrySyncFilamentToToolhead(bridge, "snap1", 0, 42); err != nil {
		t.Fatalf("TrySyncFilamentToToolhead failed: %v", err)
	}
	if calls != 0 {
		t.Fatalf("expected no Moonraker calls when sync disabled, got %d", calls)
	}
}

func TestTrySyncFilamentToToolheadClearSlot(t *testing.T) {
	bridge, err := core.NewFilamentBridge(nil)
	if err != nil {
		t.Fatalf("NewFilamentBridge failed: %v", err)
	}
	defer bridge.DB.Close()

	if err := bridge.SetSyncFilamentToPrinterEnabled(true); err != nil {
		t.Fatalf("SetSyncFilamentToPrinterEnabled failed: %v", err)
	}

	var receivedScript string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/printer/gcode/script" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var req moonrakerGcodeScriptRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("failed to decode script request: %v", err)
		}
		receivedScript = req.Script
		_ = json.NewEncoder(w).Encode(map[string]any{"result": "ok"})
	}))
	defer server.Close()

	if err := bridge.SavePrinterConfig("snap1", core.PrinterConfig{
		Name:      "Snapmaker",
		IPAddress: server.URL,
		Toolheads: 4,
	}); err != nil {
		t.Fatalf("SavePrinterConfig failed: %v", err)
	}

	bridge.Config = &core.Config{
		PrinterTimeout: core.PrinterTimeout,
		Printers: map[string]core.PrinterConfig{
			"snap1": {
				Name:      "Snapmaker",
				IPAddress: server.URL,
				Toolheads: 4,
			},
		},
	}

	if err := TrySyncFilamentToToolhead(bridge, "snap1", 1, 0); err != nil {
		t.Fatalf("TrySyncFilamentToToolhead failed: %v", err)
	}
	if !strings.Contains(receivedScript, "CONFIG_EXTRUDER=1") {
		t.Fatalf("expected extruder 1 in script, got %q", receivedScript)
	}
	if !strings.Contains(receivedScript, "VENDOR=NONE") {
		t.Fatalf("expected cleared vendor in script, got %q", receivedScript)
	}
}
