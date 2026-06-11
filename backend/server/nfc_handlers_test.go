package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"filabridge/core"
	"filabridge/spoolman"
)

func nfcTestSpoolmanStub(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/spool":
		json.NewEncoder(w).Encode([]spoolman.Spool{
			{ID: 7, RemainingWeight: 500},
		})
	case r.URL.Path == "/api/v1/setting/locations":
		json.NewEncoder(w).Encode(spoolman.SettingResponse{
			Value: `["Drybox"]`,
			IsSet: true,
			Type:  "array",
		})
	default:
		http.NotFound(w, r)
	}
}

func getNfcUrls(t *testing.T, ws *WebServer, forwardedHost string) []map[string]interface{} {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/nfc/urls", nil)
	if forwardedHost != "" {
		req.Header.Set("X-Forwarded-Host", forwardedHost)
	}
	rec := httptest.NewRecorder()
	ws.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		URLs []map[string]interface{} `json:"urls"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(body.URLs) == 0 {
		t.Fatal("expected at least one NFC URL entry")
	}
	return body.URLs
}

func TestNfcUrlsUseConfiguredPublicURL(t *testing.T) {
	bridge, ws := newTestBridgeWithSpoolman(t, nfcTestSpoolmanStub)

	// Trailing slash must be trimmed when building tag URLs.
	if err := bridge.SetConfigValue(core.ConfigKeyFilabridgePublicURL, "http://192.168.1.20:5000/"); err != nil {
		t.Fatalf("failed to set public url: %v", err)
	}

	urls := getNfcUrls(t, ws, "0.0.0.0:5000")
	for _, entry := range urls {
		u, _ := entry["url"].(string)
		if !strings.HasPrefix(u, "http://192.168.1.20:5000/api/nfc/assign") {
			t.Fatalf("expected URL based on configured public URL, got %q", u)
		}
	}
}

func TestNfcUrlsFallBackToRequestHost(t *testing.T) {
	_, ws := newTestBridgeWithSpoolman(t, nfcTestSpoolmanStub)

	urls := getNfcUrls(t, ws, "192.168.1.30:5000")
	for _, entry := range urls {
		u, _ := entry["url"].(string)
		if !strings.HasPrefix(u, "http://192.168.1.30:5000/api/nfc/assign") {
			t.Fatalf("expected URL based on request host, got %q", u)
		}
	}
}
