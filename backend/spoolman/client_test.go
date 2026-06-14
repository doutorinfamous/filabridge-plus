package spoolman

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMergeLocations(t *testing.T) {
	configured := []string{"Drybox", "Shelf A"}
	spoolDerived := []Location{
		{Name: "Shelf A"},
		{Name: "Printer"},
	}

	merged := mergeLocations(configured, spoolDerived)

	if len(merged) != 3 {
		t.Fatalf("expected 3 locations, got %d", len(merged))
	}

	expectedOrder := []string{"Drybox", "Shelf A", "Printer"}
	for i, name := range expectedOrder {
		if merged[i].Name != name {
			t.Fatalf("expected location %q at index %d, got %q", name, i, merged[i].Name)
		}
	}
}

func TestMergeLocationsSkipsArchived(t *testing.T) {
	configured := []string{"Drybox"}
	spoolDerived := []Location{
		{Name: "Arquivado", Archived: true},
		{Name: "Ativo"},
	}

	merged := mergeLocations(configured, spoolDerived)

	if len(merged) != 2 {
		t.Fatalf("expected 2 locations, got %d", len(merged))
	}
	if merged[1].Name != "Ativo" {
		t.Fatalf("expected active location, got %q", merged[1].Name)
	}
}

func TestGetLocationsMergesSettingAndSpoolLocations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/setting/locations":
			json.NewEncoder(w).Encode(SettingResponse{
				Value: `["Drybox","Shelf B"]`,
				IsSet: true,
				Type:  "array",
			})
		case "/api/v1/location":
			json.NewEncoder(w).Encode([]string{"Shelf A", "Shelf B"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, 5, "", "")
	locations, err := client.GetLocations()
	if err != nil {
		t.Fatalf("GetLocations failed: %v", err)
	}

	if len(locations) != 3 {
		t.Fatalf("expected 3 locations, got %d", len(locations))
	}

	expected := []string{"Drybox", "Shelf B", "Shelf A"}
	for i, name := range expected {
		if locations[i].Name != name {
			t.Fatalf("expected location %q at index %d, got %q", name, i, locations[i].Name)
		}
	}
}

func TestGetLocationsFallsBackWhenSettingUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/setting/locations":
			http.NotFound(w, r)
		case "/api/v1/location":
			json.NewEncoder(w).Encode([]string{"Shelf A"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, 5, "", "")
	locations, err := client.GetLocations()
	if err != nil {
		t.Fatalf("GetLocations failed: %v", err)
	}

	if len(locations) != 1 || locations[0].Name != "Shelf A" {
		t.Fatalf("expected spool-derived location only, got %+v", locations)
	}
}

func TestEnsureConfiguredLocationAddsNewName(t *testing.T) {
	var posted []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/setting/locations":
			json.NewEncoder(w).Encode(SettingResponse{
				Value: `["Drybox"]`,
				IsSet: true,
				Type:  "array",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/setting/locations":
			var names []string
			if err := json.NewDecoder(r.Body).Decode(&names); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			posted = names
			json.NewEncoder(w).Encode(SettingResponse{
				Value: `["Drybox","My Printer - Toolhead 1"]`,
				IsSet: true,
				Type:  "array",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, 5, "", "")
	if err := client.EnsureConfiguredLocation("My Printer - Toolhead 1"); err != nil {
		t.Fatalf("EnsureConfiguredLocation failed: %v", err)
	}
	if len(posted) != 2 || posted[1] != "My Printer - Toolhead 1" {
		t.Fatalf("expected posted locations with new entry, got %+v", posted)
	}
}

func TestEnsureConfiguredLocationIsIdempotent(t *testing.T) {
	postCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/setting/locations":
			json.NewEncoder(w).Encode(SettingResponse{
				Value: `["Drybox"]`,
				IsSet: true,
				Type:  "array",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/setting/locations":
			postCount++
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(SettingResponse{Value: `["Drybox"]`, IsSet: true, Type: "array"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, 5, "", "")
	if err := client.EnsureConfiguredLocation("Drybox"); err != nil {
		t.Fatalf("EnsureConfiguredLocation failed: %v", err)
	}
	if postCount != 0 {
		t.Fatalf("expected no POST when location already exists, got %d", postCount)
	}
}
