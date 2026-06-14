// Package homeassistant implements a generic Home Assistant REST + WebSocket client.
package homeassistant

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Client is a generic Home Assistant REST + WebSocket client.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client

	entityMapMu     sync.RWMutex
	entityMap       map[string]string
	entityMapExpiry time.Time
}

// State represents a Home Assistant entity state.
type State struct {
	EntityID   string                 `json:"entity_id"`
	State      string                 `json:"state"`
	Attributes map[string]interface{} `json:"attributes"`
}

// EntityRegistryEntry represents an entity registry entry.
type EntityRegistryEntry struct {
	EntityID                string            `json:"entity_id"`
	UniqueID                string            `json:"unique_id"`
	Platform                string            `json:"platform"`
	DeviceID                string            `json:"device_id"`
	DisabledBy              *string           `json:"disabled_by"`
	TranslationKey          string            `json:"translation_key"`
	TranslationPlaceholders map[string]string `json:"translation_placeholders"`
}

// DeviceRegistryEntry represents a device registry entry.
type DeviceRegistryEntry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	NameByUser  string `json:"name_by_user"`
	ViaDeviceID string `json:"via_device_id"`
}

// NewClient creates a Home Assistant API client.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) authRequest(method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.httpClient.Do(req)
}

// TestConnection verifies HA connectivity.
func (c *Client) TestConnection() error {
	resp, err := c.authRequest(http.MethodGet, "/api/", nil)
	if err != nil {
		return fmt.Errorf("cannot reach Home Assistant: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("home assistant returned HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// GetStates returns all entity states.
func (c *Client) GetStates() ([]State, error) {
	resp, err := c.authRequest(http.MethodGet, "/api/states", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get states (HTTP %d): %s", resp.StatusCode, string(body))
	}
	var states []State
	if err := json.NewDecoder(resp.Body).Decode(&states); err != nil {
		return nil, err
	}
	return states, nil
}

type wsMessage struct {
	ID      int             `json:"id,omitempty"`
	Type    string          `json:"type"`
	Success bool            `json:"success,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   interface{}     `json:"error,omitempty"`
	Message string          `json:"message,omitempty"`
}

// wsCommand runs one-shot WebSocket commands against HA.
func (c *Client) wsCommand(commands []map[string]interface{}) ([][]byte, error) {
	wsURL := strings.Replace(c.baseURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1) + "/api/websocket"

	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("websocket dial failed: %w", err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	var authMsg wsMessage
	if err := conn.ReadJSON(&authMsg); err != nil {
		return nil, err
	}
	if authMsg.Type != "auth_required" {
		return nil, fmt.Errorf("unexpected websocket message: %s", authMsg.Type)
	}
	if err := conn.WriteJSON(map[string]interface{}{"type": "auth", "access_token": c.token}); err != nil {
		return nil, err
	}
	if err := conn.ReadJSON(&authMsg); err != nil {
		return nil, err
	}
	if authMsg.Type != "auth_ok" {
		return nil, fmt.Errorf("websocket auth failed: %s", authMsg.Message)
	}

	results := make([][]byte, len(commands))
	received := 0
	for i, cmd := range commands {
		msg := map[string]interface{}{"id": i + 1}
		for k, v := range cmd {
			msg[k] = v
		}
		if err := conn.WriteJSON(msg); err != nil {
			return nil, err
		}
	}

	for received < len(commands) {
		var msg wsMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return nil, err
		}
		if msg.Type == "result" {
			if !msg.Success {
				return nil, fmt.Errorf("websocket command failed: %v", msg.Error)
			}
			idx := msg.ID - 1
			if idx >= 0 && idx < len(results) {
				results[idx] = msg.Result
				received++
			}
		}
	}
	return results, nil
}

// GetEntityAndDeviceRegistry fetches entity and device registries via WebSocket.
func (c *Client) GetEntityAndDeviceRegistry() ([]EntityRegistryEntry, []DeviceRegistryEntry, error) {
	raw, err := c.wsCommand([]map[string]interface{}{
		{"type": "config/entity_registry/list"},
		{"type": "config/device_registry/list"},
	})
	if err != nil {
		return nil, nil, err
	}
	var entities []EntityRegistryEntry
	var devices []DeviceRegistryEntry
	if err := json.Unmarshal(raw[0], &entities); err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal(raw[1], &devices); err != nil {
		return nil, nil, err
	}
	return entities, devices, nil
}

// GetEntityIdToUniqueIdMap returns a cached entity_id → unique_id map.
func (c *Client) GetEntityIdToUniqueIdMap() (map[string]string, error) {
	c.entityMapMu.RLock()
	if c.entityMap != nil && time.Now().Before(c.entityMapExpiry) {
		m := c.entityMap
		c.entityMapMu.RUnlock()
		return m, nil
	}
	c.entityMapMu.RUnlock()

	entities, _, err := c.GetEntityAndDeviceRegistry()
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(entities))
	for _, e := range entities {
		m[e.EntityID] = e.UniqueID
	}

	c.entityMapMu.Lock()
	c.entityMap = m
	c.entityMapExpiry = time.Now().Add(5 * time.Minute)
	c.entityMapMu.Unlock()
	return m, nil
}

// ResolveToUniqueID resolves entity_id to unique_id when possible.
func (c *Client) ResolveToUniqueID(entityID string, idMap map[string]string) string {
	if idMap != nil {
		if uid, ok := idMap[entityID]; ok && uid != "" {
			return uid
		}
	}
	return entityID
}
