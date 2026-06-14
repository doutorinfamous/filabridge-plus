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
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse location: %v", err)
	}
	if parsed.Path != "/nfc/scan" {
		t.Fatalf("expected redirect to /nfc/scan, got %q", location)
	}
	if parsed.Query().Get("session_id") == "" {
		t.Fatalf("expected session_id in redirect, got %q", location)
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
				"remaining_weight": 450,
				"filament": { "id": 1, "name": "Test Spool", "material": "PLA", "color_hex": "ff0000", "vendor": { "name": "BrandX" } }
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

func TestNfcAssignHandlerFilamentWithSingleSpool(t *testing.T) {
	_, ws := newTestBridgeWithSpoolman(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/spool":
			if r.URL.Query().Get("filament.id") == "5" {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`[{
					"id": 12,
					"remaining_weight": 800,
					"filament": { "id": 5, "name": "Blue PLA", "material": "PLA", "color_hex": "0000ff" }
				}]`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
		case "/api/v1/filament":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
		case "/api/v1/location":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
		default:
			http.NotFound(w, r)
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/api/nfc/assign?filament=5", nil)
	req.RemoteAddr = "192.168.1.60:12345"
	rec := httptest.NewRecorder()
	ws.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/api/nfc/session/status", nil)
	statusReq.RemoteAddr = "192.168.1.60:12345"
	statusRec := httptest.NewRecorder()
	ws.router.ServeHTTP(statusRec, statusReq)

	var resp struct {
		Active   bool `json:"active"`
		HasSpool bool `json:"has_spool"`
		SpoolID  int  `json:"spool_id"`
	}
	if err := json.Unmarshal(statusRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if !resp.Active || !resp.HasSpool || resp.SpoolID != 12 {
		t.Fatalf("expected active session with spool 12, got %+v", resp)
	}
}

func TestNfcAssignHandlerFilamentWithMultipleSpools(t *testing.T) {
	_, ws := newTestBridgeWithSpoolman(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/spool":
			if r.URL.Query().Get("filament.id") == "5" {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`[
					{"id": 12, "remaining_weight": 800, "location": "Shelf A", "filament": {"id": 5, "name": "Blue PLA", "material": "PLA", "color_hex": "0000ff"}},
					{"id": 18, "remaining_weight": 500, "location": "Printer - Toolhead 1", "filament": {"id": 5, "name": "Blue PLA", "material": "PLA", "color_hex": "0000ff"}}
				]`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
		case "/api/v1/filament/5":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id": 5, "name": "Blue PLA", "material": "PLA", "color_hex": "0000ff"}`))
		case "/api/v1/filament":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
		case "/api/v1/location":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
		default:
			http.NotFound(w, r)
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/api/nfc/assign?filament=5", nil)
	req.RemoteAddr = "192.168.1.61:12345"
	rec := httptest.NewRecorder()
	ws.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}

	redirectURL, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse redirect: %v", err)
	}
	sessionID := redirectURL.Query().Get("session_id")
	if sessionID == "" {
		t.Fatalf("expected session_id in redirect, got %q", rec.Header().Get("Location"))
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/api/nfc/session/status?session_id="+url.QueryEscape(sessionID), nil)
	statusReq.RemoteAddr = "192.168.1.61:12345"
	statusRec := httptest.NewRecorder()
	ws.router.ServeHTTP(statusRec, statusReq)

	var resp struct {
		HasSpool           bool `json:"has_spool"`
		HasPendingFilament bool `json:"has_pending_filament"`
		PendingFilament    struct {
			ID         int `json:"id"`
			Candidates []struct {
				ID int `json:"id"`
			} `json:"candidates"`
		} `json:"pending_filament"`
	}
	if err := json.Unmarshal(statusRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if resp.HasSpool || !resp.HasPendingFilament {
		t.Fatalf("expected pending filament without spool, got %+v", resp)
	}
	if resp.PendingFilament.ID != 5 || len(resp.PendingFilament.Candidates) != 2 {
		t.Fatalf("unexpected pending filament payload: %+v", resp.PendingFilament)
	}
}

func TestNfcAssignHandlerFilamentWithNoSpools(t *testing.T) {
	_, ws := newTestBridgeWithSpoolman(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/spool":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
		case "/api/v1/filament/9":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id": 9, "name": "Red PETG", "material": "PETG"}`))
		default:
			http.NotFound(w, r)
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/api/nfc/assign?filament=9", nil)
	rec := httptest.NewRecorder()
	ws.router.ServeHTTP(rec, req)

	location := rec.Header().Get("Location")
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse location: %v", err)
	}
	if parsed.Query().Get("error") != "no_spools_for_filament" {
		t.Fatalf("expected no_spools_for_filament error, got %q", location)
	}
	if parsed.Query().Get("filament_name") != "Red PETG" {
		t.Fatalf("expected filament name in redirect, got %q", location)
	}
}

func TestNfcSelectSpoolHandlerCompletesAssignment(t *testing.T) {
	spoolLocations := map[int]string{}
	_, ws := newTestBridgeWithSpoolman(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/spool":
			if r.URL.Query().Get("filament.id") == "5" {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`[
					{"id": 12, "remaining_weight": 800, "filament": {"id": 5, "name": "Blue PLA", "material": "PLA", "color_hex": "0000ff"}},
					{"id": 18, "remaining_weight": 500, "filament": {"id": 5, "name": "Blue PLA", "material": "PLA", "color_hex": "0000ff"}}
				]`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
		case "/api/v1/spool/18":
			if r.Method == http.MethodPatch {
				var payload struct {
					Location string `json:"location"`
				}
				_ = json.NewDecoder(r.Body).Decode(&payload)
				spoolLocations[18] = payload.Location
				w.WriteHeader(http.StatusOK)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			location := spoolLocations[18]
			w.Write([]byte(`{"id": 18, "remaining_weight": 500, "location": "` + location + `", "filament": {"id": 5, "name": "Blue PLA", "material": "PLA", "color_hex": "0000ff"}}`))
		case "/api/v1/filament/5":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id": 5, "name": "Blue PLA", "material": "PLA", "color_hex": "0000ff"}`))
		case "/api/v1/filament":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
		case "/api/v1/location":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
		default:
			if r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/api/v1/spool/") {
				var payload struct {
					Location string `json:"location"`
				}
				_ = json.NewDecoder(r.Body).Decode(&payload)
				if strings.HasSuffix(r.URL.Path, "/18") {
					spoolLocations[18] = payload.Location
				}
				w.WriteHeader(http.StatusOK)
				return
			}
			http.NotFound(w, r)
		}
	})

	locationName := "Drybox 1"
	assignReq := httptest.NewRequest(http.MethodGet, "/api/nfc/assign?location="+url.QueryEscape(locationName), nil)
	assignReq.RemoteAddr = "10.0.0.9:54321"
	assignRec := httptest.NewRecorder()
	ws.router.ServeHTTP(assignRec, assignReq)
	if assignRec.Code != http.StatusFound {
		t.Fatalf("location assign expected 302, got %d", assignRec.Code)
	}

	filamentReq := httptest.NewRequest(http.MethodGet, "/api/nfc/assign?filament=5", nil)
	filamentReq.RemoteAddr = "10.0.0.9:54321"
	filamentRec := httptest.NewRecorder()
	ws.router.ServeHTTP(filamentRec, filamentReq)
	if filamentRec.Code != http.StatusFound {
		t.Fatalf("filament assign expected 302, got %d", filamentRec.Code)
	}

	selectReq := httptest.NewRequest(http.MethodPost, "/api/nfc/session/select-spool", strings.NewReader(`{"spool_id":18}`))
	selectReq.RemoteAddr = "10.0.0.9:54321"
	selectReq.Header.Set("Content-Type", "application/json")
	selectRec := httptest.NewRecorder()
	ws.router.ServeHTTP(selectRec, selectReq)

	if selectRec.Code != http.StatusOK {
		t.Fatalf("select spool expected 200, got %d: %s", selectRec.Code, selectRec.Body.String())
	}

	var resp struct {
		Completed bool `json:"completed"`
		Success   struct {
			SpoolID      int    `json:"spool_id"`
			Location     string `json:"location"`
			LocationType string `json:"location_type"`
		} `json:"success"`
	}
	if err := json.Unmarshal(selectRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode select response: %v", err)
	}
	if !resp.Completed || resp.Success.SpoolID != 18 || resp.Success.Location != locationName {
		t.Fatalf("unexpected select response: %+v", resp)
	}
	if resp.Success.LocationType != "storage" {
		t.Fatalf("expected storage location type, got %q", resp.Success.LocationType)
	}

	if spoolLocations[18] != locationName {
		t.Fatalf("expected spool location %q, got %q", locationName, spoolLocations[18])
	}
}

func TestNfcSelectSpoolHandlerRejectsWrongFilament(t *testing.T) {
	_, ws := newTestBridgeWithSpoolman(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/spool":
			if r.URL.Query().Get("filament.id") == "5" {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`[{"id": 12, "remaining_weight": 800, "filament": {"id": 5, "name": "Blue PLA"}}]`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
		case "/api/v1/filament/5":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id": 5, "name": "Blue PLA", "material": "PLA"}`))
		default:
			http.NotFound(w, r)
		}
	})

	assignReq := httptest.NewRequest(http.MethodGet, "/api/nfc/assign?filament=5", nil)
	assignReq.RemoteAddr = "10.0.0.10:54321"
	assignRec := httptest.NewRecorder()
	ws.router.ServeHTTP(assignRec, assignReq)
	if assignRec.Code != http.StatusFound {
		t.Fatalf("assign expected 302, got %d", assignRec.Code)
	}

	selectReq := httptest.NewRequest(http.MethodPost, "/api/nfc/session/select-spool", strings.NewReader(`{"spool_id":99}`))
	selectReq.RemoteAddr = "10.0.0.10:54321"
	selectReq.Header.Set("Content-Type", "application/json")
	selectRec := httptest.NewRecorder()
	ws.router.ServeHTTP(selectRec, selectReq)

	if selectRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", selectRec.Code, selectRec.Body.String())
	}
}
