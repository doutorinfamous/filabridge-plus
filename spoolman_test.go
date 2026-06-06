package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMergeSpoolmanLocations(t *testing.T) {
	configured := []string{"Drybox", "Prateleira A"}
	spoolDerived := []SpoolmanLocation{
		{Name: "Prateleira A"},
		{Name: "Impressora"},
	}

	merged := mergeSpoolmanLocations(configured, spoolDerived)

	if len(merged) != 3 {
		t.Fatalf("expected 3 locations, got %d", len(merged))
	}

	expectedOrder := []string{"Drybox", "Prateleira A", "Impressora"}
	for i, name := range expectedOrder {
		if merged[i].Name != name {
			t.Fatalf("expected location %q at index %d, got %q", name, i, merged[i].Name)
		}
	}
}

func TestMergeSpoolmanLocationsSkipsArchived(t *testing.T) {
	configured := []string{"Drybox"}
	spoolDerived := []SpoolmanLocation{
		{Name: "Arquivado", Archived: true},
		{Name: "Ativo"},
	}

	merged := mergeSpoolmanLocations(configured, spoolDerived)

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
			json.NewEncoder(w).Encode(spoolmanSettingResponse{
				Value: `["Drybox","Prateleira B"]`,
				IsSet: true,
				Type:  "array",
			})
		case "/api/v1/location":
			json.NewEncoder(w).Encode([]string{"Prateleira A", "Prateleira B"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewSpoolmanClient(server.URL, 5, "", "")
	locations, err := client.GetLocations()
	if err != nil {
		t.Fatalf("GetLocations failed: %v", err)
	}

	if len(locations) != 3 {
		t.Fatalf("expected 3 locations, got %d", len(locations))
	}

	expected := []string{"Drybox", "Prateleira B", "Prateleira A"}
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
			json.NewEncoder(w).Encode([]string{"Prateleira A"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewSpoolmanClient(server.URL, 5, "", "")
	locations, err := client.GetLocations()
	if err != nil {
		t.Fatalf("GetLocations failed: %v", err)
	}

	if len(locations) != 1 || locations[0].Name != "Prateleira A" {
		t.Fatalf("expected spool-derived location only, got %+v", locations)
	}
}
