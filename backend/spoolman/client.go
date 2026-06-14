// Package spoolman implements the HTTP client for the Spoolman inventory API.
package spoolman

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Client handles communication with Spoolman API for bridge functionality.
type Client struct {
	baseURL    string
	httpClient *http.Client
	username   string
	password   string
}

// GetBaseURL returns the Spoolman base URL.
func (c *Client) GetBaseURL() string {
	return c.baseURL
}

// Spool represents a spool from Spoolman API.
type Spool struct {
	ID              int                    `json:"id"`
	Registered      string                 `json:"registered"`
	Filament        *Filament              `json:"filament"`
	RemainingWeight float64                `json:"remaining_weight"`
	InitialWeight   float64                `json:"initial_weight"`
	SpoolWeight     float64                `json:"spool_weight"`
	UsedWeight      float64                `json:"used_weight"`
	RemainingLength float64                `json:"remaining_length"`
	UsedLength      float64                `json:"used_length"`
	FirstUsed       string                 `json:"first_used"`
	LastUsed        string                 `json:"last_used"`
	Archived        bool                   `json:"archived"`
	LocationID      *int                   `json:"location_id"` // Reference to Spoolman Location entity
	Extra           map[string]interface{} `json:"extra"`

	// Computed fields for easier access
	Name     string `json:"name"`     // Computed from filament.name
	Brand    string `json:"brand"`    // Computed from filament.vendor.name
	Material string `json:"material"` // Computed from filament.material
	Location string `json:"location"` // Spool location (e.g., "Printer1 - Toolhead 1")
}

// Filament represents a filament type from Spoolman.
type Filament struct {
	ID                   int                    `json:"id"`
	Registered           string                 `json:"registered"`
	Name                 string                 `json:"name"`
	Vendor               *Vendor                `json:"vendor"`
	Material             string                 `json:"material"`
	Density              float64                `json:"density"`
	Diameter             float64                `json:"diameter"`
	Weight               float64                `json:"weight"`
	SpoolWeight          float64                `json:"spool_weight"`
	SettingsExtruderTemp int                    `json:"settings_extruder_temp"`
	SettingsBedTemp      int                    `json:"settings_bed_temp"`
	ColorHex             string                 `json:"color_hex"`
	ExternalID           string                 `json:"external_id"`
	Extra                map[string]interface{} `json:"extra"`
	Archived             bool                   `json:"archived"`
}

// Vendor represents a vendor from Spoolman.
type Vendor struct {
	ID         int                    `json:"id"`
	Registered string                 `json:"registered"`
	Name       string                 `json:"name"`
	ExternalID string                 `json:"external_id"`
	Extra      map[string]interface{} `json:"extra"`
	Archived   bool                   `json:"archived"`
}

// APIError represents an error response from Spoolman API.
type APIError struct {
	Detail string `json:"detail"`
	Title  string `json:"title"`
	Type   string `json:"type"`
}

// NewClient creates a new Spoolman client.
func NewClient(baseURL string, timeout int, username, password string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 2,
				IdleConnTimeout:     30 * time.Second,
			},
		},
		username: username,
		password: password,
	}
}

// addAuthHeader adds Basic Authentication header to the request if both username and password are provided.
func (c *Client) addAuthHeader(req *http.Request) {
	if c.username != "" && c.password != "" {
		auth := c.username + ":" + c.password
		encoded := base64.StdEncoding.EncodeToString([]byte(auth))
		req.Header.Set("Authorization", "Basic "+encoded)
	}
}

// handleAPIError handles API error responses from Spoolman.
func (c *Client) handleAPIError(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading error response body: %w", err)
	}

	var spoolmanErr APIError
	if err := json.Unmarshal(body, &spoolmanErr); err == nil && spoolmanErr.Detail != "" {
		return fmt.Errorf("spoolman API error (HTTP %d): %s - %s", resp.StatusCode, spoolmanErr.Title, spoolmanErr.Detail)
	}

	return fmt.Errorf("spoolman API error (HTTP %d): %s", resp.StatusCode, string(body))
}

// normalizeSpoolData normalizes spool data to extract information from nested structures.
func (c *Client) normalizeSpoolData(spool Spool) Spool {
	if spool.Filament != nil {
		spool.Name = spool.Filament.Name
		spool.Material = spool.Filament.Material

		if spool.Filament.Vendor != nil {
			spool.Brand = spool.Filament.Vendor.Name
		}
	}

	if spool.Name == "" {
		spool.Name = fmt.Sprintf("Spool %d", spool.ID)
	}

	return spool
}

// getSpoolDisplayName returns the display name for sorting purposes.
func (spool *Spool) getSpoolDisplayName() string {
	material := "Unknown Material"
	brand := "Unknown Brand"
	name := "Unnamed Spool"

	if spool.Filament != nil {
		if spool.Filament.Material != "" {
			material = spool.Filament.Material
		}
		if spool.Filament.Vendor != nil && spool.Filament.Vendor.Name != "" {
			brand = spool.Filament.Vendor.Name
		}
		if spool.Filament.Name != "" {
			name = spool.Filament.Name
		}
	}

	return fmt.Sprintf("%s - %s - %s", material, brand, name)
}

// GetAllSpools gets all filament spools from Spoolman.
func (c *Client) GetAllSpools() ([]Spool, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/v1/spool", nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	c.addAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error getting spools from Spoolman: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleAPIError(resp)
	}

	var spools []Spool
	if err := json.NewDecoder(resp.Body).Decode(&spools); err != nil {
		return nil, fmt.Errorf("error decoding spools from Spoolman: %w", err)
	}

	return c.filterAndSortSpools(spools), nil
}

func (c *Client) filterAndSortSpools(spools []Spool) []Spool {
	for i := range spools {
		spools[i] = c.normalizeSpoolData(spools[i])
	}

	filteredSpools := make([]Spool, 0, len(spools))
	for _, spool := range spools {
		if spool.RemainingWeight > 0 {
			filteredSpools = append(filteredSpools, spool)
		}
	}
	spools = filteredSpools

	sort.Slice(spools, func(i, j int) bool {
		nameI := spools[i].getSpoolDisplayName()
		nameJ := spools[j].getSpoolDisplayName()

		if nameI != nameJ {
			return nameI < nameJ
		}

		return spools[i].RemainingWeight < spools[j].RemainingWeight
	})

	return spools
}

// GetSpoolsByFilament returns non-empty spools for a specific filament type.
func (c *Client) GetSpoolsByFilament(filamentID int) ([]Spool, error) {
	query := url.Values{}
	query.Set("filament.id", strconv.Itoa(filamentID))

	req, err := http.NewRequest("GET", c.baseURL+"/api/v1/spool?"+query.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	c.addAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error getting spools for filament %d from Spoolman: %w", filamentID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleAPIError(resp)
	}

	var spools []Spool
	if err := json.NewDecoder(resp.Body).Decode(&spools); err != nil {
		return nil, fmt.Errorf("error decoding spools for filament %d from Spoolman: %w", filamentID, err)
	}

	return c.filterAndSortSpools(spools), nil
}

// GetFilament returns a single filament type by ID from Spoolman.
func (c *Client) GetFilament(filamentID int) (*Filament, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/filament/%d", c.baseURL, filamentID), nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	c.addAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error getting filament %d from Spoolman: %w", filamentID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("filament %d not found in Spoolman: %w", filamentID, c.handleAPIError(resp))
	}

	var filament Filament
	if err := json.NewDecoder(resp.Body).Decode(&filament); err != nil {
		return nil, fmt.Errorf("error decoding filament %d from Spoolman: %w", filamentID, err)
	}

	return &filament, nil
}

// GetAllFilaments gets all filament types from Spoolman.
func (c *Client) GetAllFilaments() ([]Filament, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/v1/filament", nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	c.addAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error getting filaments from Spoolman: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleAPIError(resp)
	}

	var filaments []Filament
	if err := json.NewDecoder(resp.Body).Decode(&filaments); err != nil {
		return nil, fmt.Errorf("error decoding filaments from Spoolman: %w", err)
	}

	filteredFilaments := make([]Filament, 0, len(filaments))
	for _, filament := range filaments {
		if !filament.Archived {
			filteredFilaments = append(filteredFilaments, filament)
		}
	}
	filaments = filteredFilaments

	sort.Slice(filaments, func(i, j int) bool {
		return filaments[i].ID < filaments[j].ID
	})

	return filaments, nil
}

// UpdateSpool updates spool information (used for filament usage tracking).
func (c *Client) UpdateSpool(spoolID int, data map[string]interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("error marshaling spool update data: %w", err)
	}

	req, err := http.NewRequest("PATCH", fmt.Sprintf("%s/api/v1/spool/%d", c.baseURL, spoolID), bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating PUT request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.addAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error updating spool %d in Spoolman: %w", spoolID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.handleAPIError(resp)
	}

	return nil
}

// GetSpool returns a single spool by ID from Spoolman.
func (c *Client) GetSpool(spoolID int) (*Spool, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/spool/%d", c.baseURL, spoolID), nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	c.addAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error getting spool %d from Spoolman: %w", spoolID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("spool %d not found in Spoolman: %w", spoolID, c.handleAPIError(resp))
	}

	var spool Spool
	if err := json.NewDecoder(resp.Body).Decode(&spool); err != nil {
		return nil, fmt.Errorf("error decoding spool %d from Spoolman: %w", spoolID, err)
	}

	normalized := c.normalizeSpoolData(spool)
	return &normalized, nil
}

// UpdateSpoolUsage updates spool used weight based on usage (core bridge functionality).
func (c *Client) UpdateSpoolUsage(spoolID int, filamentUsed float64) error {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/spool/%d", c.baseURL, spoolID), nil)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	c.addAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error getting spool %d from Spoolman: %w", spoolID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("spool %d not found in Spoolman: %w", spoolID, c.handleAPIError(resp))
	}

	var spool Spool
	if err := json.NewDecoder(resp.Body).Decode(&spool); err != nil {
		return fmt.Errorf("error decoding spool %d from Spoolman: %w", spoolID, err)
	}

	newUsedWeight := spool.UsedWeight + filamentUsed
	currentTime := time.Now().UTC().Format(time.RFC3339)

	updateData := map[string]interface{}{
		"used_weight": newUsedWeight,
		"last_used":   currentTime,
	}

	if spool.FirstUsed == "" {
		updateData["first_used"] = currentTime
	}

	if err := c.UpdateSpool(spoolID, updateData); err != nil {
		return fmt.Errorf("failed to update spool %d: %w", spoolID, err)
	}

	fmt.Printf("Updated spool %d: used_weight %.2fg -> %.2fg (added %.2fg)\n",
		spoolID, spool.UsedWeight, newUsedWeight, filamentUsed)

	return nil
}

// TestConnection tests the connection to Spoolman.
func (c *Client) TestConnection() error {
	req, err := http.NewRequest("GET", c.baseURL+"/api/v1/info", nil)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	c.addAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error testing connection to Spoolman: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.handleAPIError(resp)
	}

	return nil
}

// Location represents a location from Spoolman.
type Location struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Comment  string `json:"comment"`
	Archived bool   `json:"archived"`
}

// SettingResponse represents a Spoolman setting API response.
type SettingResponse struct {
	Value string `json:"value"`
	IsSet bool   `json:"is_set"`
	Type  string `json:"type"`
}

// getConfiguredLocationNames returns location names stored in Spoolman's "locations" setting.
// These include empty locations created on the Locations page.
func (c *Client) getConfiguredLocationNames() ([]string, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/v1/setting/locations", nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	c.addAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error getting locations setting from Spoolman: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return []string{}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleAPIError(resp)
	}

	var setting SettingResponse
	if err := json.NewDecoder(resp.Body).Decode(&setting); err != nil {
		return nil, fmt.Errorf("error decoding locations setting from Spoolman: %w", err)
	}

	var names []string
	if err := json.Unmarshal([]byte(setting.Value), &names); err != nil {
		return nil, fmt.Errorf("error parsing locations setting value from Spoolman: %w", err)
	}

	filtered := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" {
			filtered = append(filtered, name)
		}
	}
	return filtered, nil
}

// getSpoolDerivedLocations returns locations that appear on at least one spool.
func (c *Client) getSpoolDerivedLocations() ([]Location, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/v1/location", nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	c.addAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error getting locations from Spoolman: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleAPIError(resp)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading locations response from Spoolman: %w", err)
	}

	var locations []Location
	if err := json.Unmarshal(bodyBytes, &locations); err == nil && len(locations) > 0 && locations[0].Name != "" {
		return locations, nil
	}

	var dataWrapper struct {
		Data    []Location `json:"data"`
		Results []Location `json:"results"`
	}
	if err := json.Unmarshal(bodyBytes, &dataWrapper); err == nil {
		if len(dataWrapper.Data) > 0 {
			return dataWrapper.Data, nil
		}
		if len(dataWrapper.Results) > 0 {
			return dataWrapper.Results, nil
		}
	}

	var names []string
	if err := json.Unmarshal(bodyBytes, &names); err == nil {
		for _, n := range names {
			n = strings.TrimSpace(n)
			if n != "" {
				locations = append(locations, Location{Name: n})
			}
		}
		return locations, nil
	}

	snippet := string(bodyBytes)
	if len(snippet) > 300 {
		snippet = snippet[:300] + "..."
	}
	log.Printf("Spoolman /location unexpected JSON. Snippet: %s", snippet)
	return nil, fmt.Errorf("error decoding locations from Spoolman: unexpected JSON shape")
}

func mergeLocations(configured []string, spoolDerived []Location) []Location {
	seen := make(map[string]bool)
	merged := make([]Location, 0, len(configured)+len(spoolDerived))

	for _, name := range configured {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		merged = append(merged, Location{Name: name})
	}

	for _, location := range spoolDerived {
		if location.Archived {
			continue
		}
		name := strings.TrimSpace(location.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		merged = append(merged, location)
	}

	return merged
}

// GetLocations gets all locations from Spoolman, including empty locations
// configured on the Locations page and locations currently assigned to spools.
func (c *Client) GetLocations() ([]Location, error) {
	configured, err := c.getConfiguredLocationNames()
	if err != nil {
		log.Printf("Warning: Failed to get configured locations from Spoolman settings: %v", err)
		configured = []string{}
	}

	spoolDerived, err := c.getSpoolDerivedLocations()
	if err != nil {
		return nil, err
	}

	return mergeLocations(configured, spoolDerived), nil
}

// setConfiguredLocations writes the full locations list to Spoolman settings.
func (c *Client) setConfiguredLocations(names []string) error {
	jsonData, err := json.Marshal(names)
	if err != nil {
		return fmt.Errorf("failed to marshal locations list: %w", err)
	}

	req, err := http.NewRequest("POST", c.baseURL+"/api/v1/setting/locations", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating POST request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.addAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error setting locations in Spoolman: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.handleAPIError(resp)
	}

	return nil
}

// EnsureConfiguredLocation adds a location name to Spoolman settings if not already present.
func (c *Client) EnsureConfiguredLocation(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("location name cannot be empty")
	}

	names, err := c.getConfiguredLocationNames()
	if err != nil {
		return fmt.Errorf("failed to get configured locations: %w", err)
	}

	for _, existing := range names {
		if existing == name {
			return nil
		}
	}

	names = append(names, name)
	if err := c.setConfiguredLocations(names); err != nil {
		return fmt.Errorf("failed to add location '%s' to Spoolman settings: %w", name, err)
	}

	log.Printf("Added Spoolman configured location '%s'", name)
	return nil
}

// GetOrCreateLocation gets an existing location by name.
// Note: Spoolman API does not support creating locations via POST.
// Locations must be created manually in Spoolman UI or are auto-created when referenced in spools.
func (c *Client) GetOrCreateLocation(name string) (*Location, error) {
	locations, err := c.GetLocations()
	if err != nil {
		return nil, fmt.Errorf("failed to get locations: %w", err)
	}

	for _, location := range locations {
		if location.Name == name {
			return &location, nil
		}
	}

	// Location doesn't exist in Spoolman; it will be auto-created when referenced in a spool.
	return &Location{
		ID:   0,
		Name: name,
	}, nil
}

// FindLocationByName searches for an existing location by name.
func (c *Client) FindLocationByName(name string) (*Location, error) {
	locations, err := c.GetLocations()
	if err != nil {
		return nil, fmt.Errorf("error getting locations: %w", err)
	}

	for _, location := range locations {
		if location.Name == name {
			return &location, nil
		}
	}

	return nil, nil // Location not found
}

// LocationExists checks if a location exists in Spoolman.
func (c *Client) LocationExists(name string) (bool, error) {
	location, err := c.FindLocationByName(name)
	if err != nil {
		return false, err
	}
	return location != nil, nil
}

// RenameLocation renames a location in Spoolman using the PATCH API.
func (c *Client) RenameLocation(oldName, newName string) error {
	updateData := map[string]interface{}{
		"name": newName,
	}

	jsonData, err := json.Marshal(updateData)
	if err != nil {
		return fmt.Errorf("failed to marshal location rename data: %w", err)
	}

	req, err := http.NewRequest("PATCH", fmt.Sprintf("%s/api/v1/location/%s", c.baseURL, oldName), bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating PATCH request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.addAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error renaming location in Spoolman: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.handleAPIError(resp)
	}

	log.Printf("Successfully renamed Spoolman location from '%s' to '%s'", oldName, newName)
	return nil
}

// UpdateSpoolLocation updates a spool's location in Spoolman using text-based location field.
func (c *Client) UpdateSpoolLocation(spoolID int, locationName string) error {
	return c.updateSpoolLocationText(spoolID, locationName)
}

// UpdateLocation updates a location name in Spoolman.
func (c *Client) UpdateLocation(locationID int, newName string) error {
	updateData := map[string]interface{}{
		"name": newName,
	}

	jsonData, err := json.Marshal(updateData)
	if err != nil {
		return fmt.Errorf("failed to marshal location update data: %w", err)
	}

	req, err := http.NewRequest("PATCH", fmt.Sprintf("%s/api/v1/location/%d", c.baseURL, locationID), bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating PATCH request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.addAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error updating location %d in Spoolman: %w", locationID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.handleAPIError(resp)
	}

	log.Printf("Successfully updated Spoolman location %d to '%s'", locationID, newName)
	return nil
}

// ArchiveLocation archives a location in Spoolman.
func (c *Client) ArchiveLocation(locationID int) error {
	updateData := map[string]interface{}{
		"archived": true,
	}

	jsonData, err := json.Marshal(updateData)
	if err != nil {
		return fmt.Errorf("failed to marshal location archive data: %w", err)
	}

	req, err := http.NewRequest("PATCH", fmt.Sprintf("%s/api/v1/location/%d", c.baseURL, locationID), bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating PATCH request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.addAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error archiving location %d in Spoolman: %w", locationID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.handleAPIError(resp)
	}

	log.Printf("Successfully archived Spoolman location %d", locationID)
	return nil
}

// UpdateLocationByName updates a location in Spoolman by name.
func (c *Client) UpdateLocationByName(oldName, newName string) error {
	locations, err := c.GetLocations()
	if err != nil {
		return fmt.Errorf("failed to get locations: %w", err)
	}

	var locationID int
	found := false
	for _, loc := range locations {
		if loc.Name == oldName && !loc.Archived {
			locationID = loc.ID
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("location '%s' not found in Spoolman", oldName)
	}

	return c.UpdateLocation(locationID, newName)
}

// updateSpoolLocationText updates a spool's location using the text field.
func (c *Client) updateSpoolLocationText(spoolID int, locationName string) error {
	updateData := map[string]interface{}{
		"location": locationName,
	}

	jsonData, err := json.Marshal(updateData)
	if err != nil {
		return fmt.Errorf("error marshaling location update data: %w", err)
	}

	req, err := http.NewRequest("PATCH", fmt.Sprintf("%s/api/v1/spool/%d", c.baseURL, spoolID), bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating PATCH request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.addAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error updating spool %d location in Spoolman: %w", spoolID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.handleAPIError(resp)
	}

	log.Printf("Successfully updated spool %d to location '%s' (text-based)", spoolID, locationName)
	return nil
}
