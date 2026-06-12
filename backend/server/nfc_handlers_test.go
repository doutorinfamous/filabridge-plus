package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"filabridge/core"
)

func TestNfcUrlsHandlerIncludesAllToolheads(t *testing.T) {
	bridge, ws := newTestBridgeWithSpoolman(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/spool":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
		case "/api/v1/location":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
		case "/api/v1/setting/locations":
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	})

	if err := bridge.SavePrinterConfig("snapmaker1", core.PrinterConfig{
		Name:      "Snapmaker U1",
		Driver:    core.DriverMoonraker,
		Toolheads: 4,
	}); err != nil {
		t.Fatalf("SavePrinterConfig failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/nfc/urls", nil)
	rec := httptest.NewRecorder()
	ws.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		URLs []map[string]interface{} `json:"urls"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	toolheadCount := 0
	for _, entry := range resp.URLs {
		if entry["type"] != "location" {
			continue
		}
		if entry["location_type"] != "toolhead" {
			continue
		}
		if entry["printer_name"] != "Snapmaker U1" {
			continue
		}
		toolheadCount++
	}
	if toolheadCount != 4 {
		t.Fatalf("expected 4 toolhead location entries for Snapmaker U1, got %d", toolheadCount)
	}
}
