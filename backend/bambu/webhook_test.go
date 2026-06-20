package bambu

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"filabridge/core"
	"filabridge/spoolman"
)

type webhookSpoolStore struct {
	mu        sync.Mutex
	spools    map[int]*spoolman.Spool
	useCalls  []float64
}

func newWebhookSpoolStore(initial ...spoolman.Spool) *webhookSpoolStore {
	s := &webhookSpoolStore{spools: make(map[int]*spoolman.Spool)}
	for i := range initial {
		copy := initial[i]
		s.spools[copy.ID] = &copy
	}
	return s
}

func (s *webhookSpoolStore) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()

		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/spool/"):
			id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/v1/spool/"))
			sp, ok := s.spools[id]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(sp)
		case strings.HasSuffix(r.URL.Path, "/use") && r.Method == http.MethodPut:
			parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/spool/"), "/")
			id, _ := strconv.Atoi(parts[0])
			sp, ok := s.spools[id]
			if !ok {
				http.NotFound(w, r)
				return
			}
			var payload struct {
				UseWeight float64 `json:"use_weight"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			sp.UsedWeight += payload.UseWeight
			sp.RemainingWeight -= payload.UseWeight
			s.useCalls = append(s.useCalls, payload.UseWeight)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(sp)
		default:
			http.NotFound(w, r)
		}
	}
}

func newWebhookTestBridge(t *testing.T, store *webhookSpoolStore) *core.FilamentBridge {
	t.Helper()

	server := httptest.NewServer(store.handler())
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

	if err := bridge.SetConfigValue(core.ConfigKeySpoolmanURL, server.URL); err != nil {
		t.Fatalf("failed to set spoolman url: %v", err)
	}
	if err := bridge.SavePrinterConfig("printer1", core.PrinterConfig{
		Name:      "My Bambu",
		Driver:    core.DriverBambuHA,
		HAPrefix:  "03919c461204338",
		IPAddress: "127.0.0.1",
		Toolheads: 1,
	}); err != nil {
		t.Fatalf("failed to save printer config: %v", err)
	}
	bridge.Config = &core.Config{
		SpoolmanURL: server.URL,
		Printers: map[string]core.PrinterConfig{
			"printer1": {
				Name:     "My Bambu",
				Driver:   core.DriverBambuHA,
				HAPrefix: "03919c461204338",
			},
		},
	}
	return bridge
}

func TestProcessSpoolUsageDebitsImmediately(t *testing.T) {
	store := newWebhookSpoolStore(spoolman.Spool{
		ID:              7,
		RemainingWeight: 500,
	})
	bridge := newWebhookTestBridge(t, store)

	if err := bridge.SetSlotSpool("tray_a", "printer1", core.SlotTypeAMSTray, "AMS Slot 1", 7); err != nil {
		t.Fatalf("SetSlotSpool failed: %v", err)
	}

	result := ProcessWebhook(bridge, WebhookPayload{
		Event:        "spool_usage",
		ActiveTrayID: "tray_a",
		UsedWeight:   3.25,
		PrinterPrefix: "03919c461204338",
		JobName:      "test.3mf",
	}, nil)

	if result.Status != "success" {
		t.Fatalf("expected success, got %+v", result)
	}
	if result.Deducted != 3.25 {
		t.Fatalf("expected 3.25g deducted, got %v", result.Deducted)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.useCalls) != 1 || store.useCalls[0] != 3.25 {
		t.Fatalf("expected immediate Spoolman use call, got %v", store.useCalls)
	}
}
