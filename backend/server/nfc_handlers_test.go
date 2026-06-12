package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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

func TestNfcAssignHandlerRedirectsToScanPage(t *testing.T) {
	_, ws := newTestBridgeWithSpoolman(t, func(w http.ResponseWriter, r *http.Request) {
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

	req := httptest.NewRequest(http.MethodGet, "/api/nfc/assign?spool=42", nil)
	req.RemoteAddr = "192.168.1.50:12345"
	rec := httptest.NewRecorder()
	ws.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	if location != "/nfc/scan" {
		t.Fatalf("expected redirect to /nfc/scan, got %q", location)
	}
}

func TestNfcAssignHandlerRedirectsErrorToScanPage(t *testing.T) {
	_, ws := newTestBridgeWithSpoolman(t, func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/nfc/assign?spool=not-a-number", nil)
	rec := httptest.NewRecorder()
	ws.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	location := rec.Header().Get("Location")
	if !strings.HasPrefix(location, "/nfc/scan?") {
		t.Fatalf("expected error redirect to /nfc/scan, got %q", location)
	}
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse location: %v", err)
	}
	if parsed.Query().Get("error") == "" {
		t.Fatalf("expected error query param, got %q", location)
	}
}

func TestNfcSessionStatusHandlerEnrichedSpool(t *testing.T) {
	_, ws := newTestBridgeWithSpoolman(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/spool/7":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"id": 7,
				"name": "Test Spool",
				"material": "PLA",
				"brand": "BrandX",
				"remaining_weight": 450,
				"filament": { "id": 1, "name": "Red PLA", "color_hex": "ff0000" }
			}`))
		case "/api/v1/spool":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
		case "/api/v1/location":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
		default:
			http.NotFound(w, r)
		}
	})

	assignReq := httptest.NewRequest(http.MethodGet, "/api/nfc/assign?spool=7", nil)
	assignReq.RemoteAddr = "10.0.0.8:54321"
	assignRec := httptest.NewRecorder()
	ws.router.ServeHTTP(assignRec, assignReq)
	if assignRec.Code != http.StatusFound {
		t.Fatalf("assign expected 302, got %d", assignRec.Code)
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/api/nfc/session/status", nil)
	statusReq.RemoteAddr = "10.0.0.8:54321"
	statusRec := httptest.NewRecorder()
	ws.router.ServeHTTP(statusRec, statusReq)

	if statusRec.Code != http.StatusOK {
		t.Fatalf("status expected 200, got %d: %s", statusRec.Code, statusRec.Body.String())
	}

	var resp struct {
		Active    bool `json:"active"`
		HasSpool  bool `json:"has_spool"`
		HasLocation bool `json:"has_location"`
		Spool     struct {
			ID              int     `json:"id"`
			Name            string  `json:"name"`
			Material        string  `json:"material"`
			Brand           string  `json:"brand"`
			ColorHex        string  `json:"color_hex"`
			RemainingWeight float64 `json:"remaining_weight"`
		} `json:"spool"`
	}
	if err := json.Unmarshal(statusRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if !resp.Active || !resp.HasSpool || resp.HasLocation {
		t.Fatalf("unexpected session flags: %+v", resp)
	}
	if resp.Spool.ID != 7 || resp.Spool.Name != "Test Spool" {
		t.Fatalf("unexpected spool meta: %+v", resp.Spool)
	}
	if resp.Spool.ColorHex != "#ff0000" {
		t.Fatalf("expected normalized color hex, got %q", resp.Spool.ColorHex)
	}
}
