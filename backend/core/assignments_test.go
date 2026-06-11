package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"filabridge/spoolman"
)

type spoolStore struct {
	mu     sync.Mutex
	spools map[int]*spoolman.Spool
}

func newSpoolStore(initial ...spoolman.Spool) *spoolStore {
	s := &spoolStore{spools: make(map[int]*spoolman.Spool)}
	for i := range initial {
		copy := initial[i]
		s.spools[copy.ID] = &copy
	}
	return s
}

func (s *spoolStore) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/spool":
			var all []spoolman.Spool
			for _, sp := range s.spools {
				if sp.RemainingWeight > 0 {
					all = append(all, *sp)
				}
			}
			json.NewEncoder(w).Encode(all)
		case strings.HasPrefix(r.URL.Path, "/api/v1/spool/") && r.Method == http.MethodGet:
			id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/v1/spool/"))
			sp, ok := s.spools[id]
			if !ok {
				http.NotFound(w, r)
				return
			}
			json.NewEncoder(w).Encode(sp)
		case strings.HasPrefix(r.URL.Path, "/api/v1/spool/") && r.Method == http.MethodPatch:
			id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/v1/spool/"))
			sp, ok := s.spools[id]
			if !ok {
				http.NotFound(w, r)
				return
			}
			var patch map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if loc, ok := patch["location"].(string); ok {
				sp.Location = loc
			}
			if extra, ok := patch["extra"].(map[string]interface{}); ok {
				if sp.Extra == nil {
					sp.Extra = make(map[string]interface{})
				}
				for k, v := range extra {
					sp.Extra[k] = v
				}
			}
			json.NewEncoder(w).Encode(sp)
		case strings.HasPrefix(r.URL.Path, "/api/v1/field/spool/"):
			w.WriteHeader(http.StatusOK)
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
	}
}

func (s *spoolStore) activeTray(spoolID int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	sp := s.spools[spoolID]
	if sp == nil {
		return ""
	}
	return spoolman.GetSpoolExtraString(sp, spoolman.ExtraFieldActiveTray)
}

func newAssignmentTestBridge(t *testing.T, store *spoolStore) *FilamentBridge {
	t.Helper()

	server := httptest.NewServer(store.handler())
	t.Cleanup(server.Close)

	dir := t.TempDir()
	bridge, err := NewFilamentBridge(&Config{
		DBFile:      filepath.Join(dir, "test.db"),
		SpoolmanURL: server.URL,
	})
	if err != nil {
		t.Fatalf("failed to create bridge: %v", err)
	}
	t.Cleanup(func() { bridge.Close() })

	if err := bridge.SetConfigValue(ConfigKeySpoolmanURL, server.URL); err != nil {
		t.Fatalf("failed to set spoolman url: %v", err)
	}
	if err := bridge.SavePrinterConfig("printer1", PrinterConfig{
		Name:      "My Printer",
		IPAddress: "127.0.0.1",
		Toolheads: 2,
	}); err != nil {
		t.Fatalf("failed to save printer config: %v", err)
	}
	return bridge
}

func TestSetToolheadMappingClearsBambuTray(t *testing.T) {
	store := newSpoolStore(spoolman.Spool{
		ID:              1,
		RemainingWeight: 500,
		Extra: map[string]interface{}{
			spoolman.ExtraFieldActiveTray: "tray_a",
		},
	})
	bridge := newAssignmentTestBridge(t, store)

	if err := bridge.SetToolheadMapping("printer1", 0, 1); err != nil {
		t.Fatalf("SetToolheadMapping failed: %v", err)
	}

	mapped, err := bridge.GetToolheadMapping("printer1", 0)
	if err != nil {
		t.Fatalf("GetToolheadMapping failed: %v", err)
	}
	if mapped != 1 {
		t.Fatalf("expected spool 1 mapped to toolhead 0, got %d", mapped)
	}
	if tray := store.activeTray(1); tray != "" {
		t.Fatalf("expected active_tray cleared, got %q", tray)
	}
}

func TestSetToolheadMappingMovesBetweenToolheads(t *testing.T) {
	store := newSpoolStore(spoolman.Spool{ID: 1, RemainingWeight: 500})
	bridge := newAssignmentTestBridge(t, store)

	if err := bridge.SetToolheadMapping("printer1", 0, 1); err != nil {
		t.Fatalf("first SetToolheadMapping failed: %v", err)
	}
	if err := bridge.SetToolheadMapping("printer1", 1, 1); err != nil {
		t.Fatalf("second SetToolheadMapping failed: %v", err)
	}

	toolhead0, err := bridge.GetToolheadMapping("printer1", 0)
	if err != nil {
		t.Fatalf("GetToolheadMapping toolhead 0 failed: %v", err)
	}
	if toolhead0 != 0 {
		t.Fatalf("expected toolhead 0 unmapped, got spool %d", toolhead0)
	}

	toolhead1, err := bridge.GetToolheadMapping("printer1", 1)
	if err != nil {
		t.Fatalf("GetToolheadMapping toolhead 1 failed: %v", err)
	}
	if toolhead1 != 1 {
		t.Fatalf("expected spool 1 on toolhead 1, got %d", toolhead1)
	}
}

func TestAssignSpoolToLocationStorageClearsBambuTray(t *testing.T) {
	store := newSpoolStore(spoolman.Spool{
		ID:              1,
		RemainingWeight: 500,
		Extra: map[string]interface{}{
			spoolman.ExtraFieldActiveTray: "tray_a",
		},
	})
	bridge := newAssignmentTestBridge(t, store)

	if err := bridge.AssignSpoolToLocation(1, "", 0, "Drybox", false); err != nil {
		t.Fatalf("AssignSpoolToLocation failed: %v", err)
	}
	if tray := store.activeTray(1); tray != "" {
		t.Fatalf("expected active_tray cleared, got %q", tray)
	}
}
