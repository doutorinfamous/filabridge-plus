package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"filabridge/core"
	"filabridge/spoolman"
)

func newTestBridgeWithSpoolman(t *testing.T, handler http.HandlerFunc) (*core.FilamentBridge, *WebServer) {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	dir := t.TempDir()
	bridge, err := core.NewFilamentBridge(&core.Config{
		DBFile:      filepath.Join(dir, "test.db"),
		SpoolmanURL: server.URL,
	})
	if err != nil {
		t.Fatalf("failed to create bridge: %v", err)
	}
	t.Cleanup(func() { bridge.Close() })

	// Persist the stub URL so ReloadConfig (triggered by printer CRUD handlers)
	// keeps pointing at the test server instead of the default Spoolman URL.
	if err := bridge.SetConfigValue(core.ConfigKeySpoolmanURL, server.URL); err != nil {
		t.Fatalf("failed to set spoolman url: %v", err)
	}

	if err := bridge.SavePrinterConfig("printer1", core.PrinterConfig{
		Name:      "My Printer",
		IPAddress: "127.0.0.1",
		Toolheads: 2,
	}); err != nil {
		t.Fatalf("failed to save printer config: %v", err)
	}

	return bridge, NewWebServer(bridge)
}

func postMapToolhead(t *testing.T, ws *WebServer, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/map_toolhead", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	ws.router.ServeHTTP(rec, req)
	return rec
}

func TestMapToolheadHandlerUpdatesSpoolmanLocation(t *testing.T) {
	var patchedSpoolID int
	var patchedLocation string

	_, ws := newTestBridgeWithSpoolman(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/api/v1/spool/"):
			idStr := strings.TrimPrefix(r.URL.Path, "/api/v1/spool/")
			patchedSpoolID, _ = strconv.Atoi(idStr)

			var update map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if loc, ok := update["location"].(string); ok {
				patchedLocation = loc
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{})
		case r.URL.Path == "/api/v1/setting/locations":
			http.NotFound(w, r)
		case r.URL.Path == "/api/v1/location":
			json.NewEncoder(w).Encode([]string{})
		default:
			http.NotFound(w, r)
		}
	})

	rec := postMapToolhead(t, ws, `{"printer_name":"My Printer","toolhead_id":0,"spool_id":42}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if patchedSpoolID != 42 {
		t.Fatalf("expected Spoolman PATCH for spool 42, got %d", patchedSpoolID)
	}
	if patchedLocation != "My Printer - Toolhead 1" {
		t.Fatalf("expected Spoolman location %q, got %q", "My Printer - Toolhead 1", patchedLocation)
	}
}

func TestMapToolheadHandlerUnmapAutoAssignsToStorage(t *testing.T) {
	type spoolPatch struct {
		spoolID  int
		location string
	}
	var patches []spoolPatch

	bridge, ws := newTestBridgeWithSpoolman(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/api/v1/spool/"):
			idStr := strings.TrimPrefix(r.URL.Path, "/api/v1/spool/")
			spoolID, _ := strconv.Atoi(idStr)

			var update map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			loc, _ := update["location"].(string)
			patches = append(patches, spoolPatch{spoolID: spoolID, location: loc})
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{})
		case r.URL.Path == "/api/v1/setting/locations":
			json.NewEncoder(w).Encode(spoolman.SettingResponse{
				Value: `["Drybox"]`,
				IsSet: true,
				Type:  "array",
			})
		case r.URL.Path == "/api/v1/location":
			json.NewEncoder(w).Encode([]string{"Drybox"})
		default:
			http.NotFound(w, r)
		}
	})

	if err := bridge.SetAutoAssignPreviousSpoolEnabled(true); err != nil {
		t.Fatalf("failed to enable auto-assign: %v", err)
	}
	if err := bridge.SetAutoAssignPreviousSpoolLocation("Drybox"); err != nil {
		t.Fatalf("failed to set auto-assign location: %v", err)
	}
	if err := bridge.SetToolheadMapping("printer1", 0, 10); err != nil {
		t.Fatalf("failed to set initial mapping: %v", err)
	}

	rec := postMapToolhead(t, ws, `{"printer_name":"My Printer","toolhead_id":0,"spool_id":0}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	spoolID, err := bridge.GetToolheadMapping("printer1", 0)
	if err != nil {
		t.Fatalf("GetToolheadMapping failed: %v", err)
	}
	if spoolID != 0 {
		t.Fatalf("expected toolhead unmapped, got spool %d", spoolID)
	}

	foundStoragePatch := false
	for _, p := range patches {
		if p.spoolID == 10 && p.location == "Drybox" {
			foundStoragePatch = true
		}
	}
	if !foundStoragePatch {
		t.Fatalf("expected Spoolman PATCH moving spool 10 to Drybox, got %+v", patches)
	}
}

func TestAddPrinterHandlerCreatesSpoolmanToolheadLocations(t *testing.T) {
	configured := []string{}

	_, ws := newTestBridgeWithSpoolman(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/setting/locations":
			value, _ := json.Marshal(configured)
			json.NewEncoder(w).Encode(spoolman.SettingResponse{
				Value: string(value),
				IsSet: true,
				Type:  "array",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/setting/locations":
			if err := json.NewDecoder(r.Body).Decode(&configured); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
			value, _ := json.Marshal(configured)
			json.NewEncoder(w).Encode(spoolman.SettingResponse{Value: string(value), IsSet: true, Type: "array"})
		default:
			http.NotFound(w, r)
		}
	})

	body := `{"name":"XL","model":"Unknown","ip_address":"192.168.1.10","api_key":"","toolheads":2}`
	req := httptest.NewRequest(http.MethodPost, "/api/printers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	ws.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	expected := []string{"XL - Toolhead 1", "XL - Toolhead 2"}
	if len(configured) != len(expected) {
		t.Fatalf("expected %d configured locations, got %+v", len(expected), configured)
	}
	for i, name := range expected {
		if configured[i] != name {
			t.Fatalf("expected location %q at index %d, got %q", name, i, configured[i])
		}
	}
}
