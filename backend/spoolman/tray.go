package spoolman

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Spool extra field keys used by FilaBridge (Bambu AMS tray tracking).
const (
	ExtraFieldActiveTray = "active_tray"
	ExtraFieldTag        = "tag"
	ExtraFieldBarcode    = "barcode"
)

var requiredSpoolExtraFields = []struct {
	key  string
	name string
}{
	{ExtraFieldActiveTray, "Active Tray"},
	{ExtraFieldTag, "RFID Tag"},
	{ExtraFieldBarcode, "Barcode"},
}

// EnsureSpoolExtraFields creates required extra fields in Spoolman if missing.
func (c *Client) EnsureSpoolExtraFields() error {
	for _, field := range requiredSpoolExtraFields {
		req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/field/spool/%s", c.baseURL, field.key), bytes.NewBuffer([]byte(
			fmt.Sprintf(`{"name":%q,"field_type":"text"}`, field.name),
		)))
		if err != nil {
			return fmt.Errorf("error creating extra field request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		c.addAuthHeader(req)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("error ensuring extra field %s: %w", field.key, err)
		}
		resp.Body.Close()
		// 200 = created, 409 = already exists
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("failed to ensure extra field %s (HTTP %d): %s", field.key, resp.StatusCode, string(body))
		}
	}
	return nil
}

func (c *Client) buildExtraMap(spool *Spool) map[string]string {
	extra := make(map[string]string)
	if spool.Extra != nil {
		for key, value := range spool.Extra {
			switch v := value.(type) {
			case string:
				extra[key] = v
			default:
				b, err := json.Marshal(v)
				if err == nil {
					extra[key] = string(b)
				}
			}
		}
	}
	return extra
}

// GetSpoolsByTray returns spools assigned to a tray unique_id.
func (c *Client) GetSpoolsByTray(trayUniqueID string) ([]Spool, error) {
	spools, err := c.GetAllSpools()
	if err != nil {
		return nil, err
	}
	jsonTrayID := JSONStringifyExtraValue(trayUniqueID)
	var matched []Spool
	for _, spool := range spools {
		if GetSpoolExtraString(&spool, ExtraFieldActiveTray) == trayUniqueID ||
			(spool.Extra != nil && spool.Extra[ExtraFieldActiveTray] == jsonTrayID) {
			matched = append(matched, spool)
		}
	}
	return matched, nil
}

// AssignSpoolToTray assigns a spool to a HA tray unique_id.
func (c *Client) AssignSpoolToTray(spoolID int, trayUniqueID string) error {
	current, err := c.GetSpoolsByTray(trayUniqueID)
	if err != nil {
		return err
	}
	for _, spool := range current {
		if spool.ID != spoolID {
			if err := c.UnassignSpoolFromTray(spool.ID); err != nil {
				return err
			}
		}
	}

	spool, err := c.GetSpool(spoolID)
	if err != nil {
		return err
	}
	extra := c.buildExtraMap(spool)
	extra[ExtraFieldActiveTray] = JSONStringifyExtraValue(trayUniqueID)
	return c.patchSpoolExtra(spoolID, extra)
}

// UnassignSpoolFromTray clears active_tray on a spool.
func (c *Client) UnassignSpoolFromTray(spoolID int) error {
	spool, err := c.GetSpool(spoolID)
	if err != nil {
		return err
	}
	extra := c.buildExtraMap(spool)
	extra[ExtraFieldActiveTray] = JSONStringifyExtraValue("")
	return c.patchSpoolExtra(spoolID, extra)
}

// SetSpoolTag stores RFID tray_uuid on a spool.
func (c *Client) SetSpoolTag(spoolID int, trayUUID string) error {
	if err := c.clearDuplicateTags(trayUUID, spoolID); err != nil {
		return err
	}
	spool, err := c.GetSpool(spoolID)
	if err != nil {
		return err
	}
	extra := c.buildExtraMap(spool)
	extra[ExtraFieldTag] = JSONStringifyExtraValue(trayUUID)
	return c.patchSpoolExtra(spoolID, extra)
}

func (c *Client) clearDuplicateTags(trayUUID string, exceptSpoolID int) error {
	spools, err := c.GetAllSpools()
	if err != nil {
		return err
	}
	for _, spool := range spools {
		if spool.ID == exceptSpoolID {
			continue
		}
		if GetSpoolExtraString(&spool, ExtraFieldTag) == trayUUID {
			if err := c.clearSpoolTag(spool.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Client) clearSpoolTag(spoolID int) error {
	spool, err := c.GetSpool(spoolID)
	if err != nil {
		return err
	}
	extra := c.buildExtraMap(spool)
	extra[ExtraFieldTag] = JSONStringifyExtraValue("")
	return c.patchSpoolExtra(spoolID, extra)
}

// FindSpoolByTag finds a spool by RFID tag.
func (c *Client) FindSpoolByTag(trayUUID string) (*Spool, error) {
	spools, err := c.GetAllSpools()
	if err != nil {
		return nil, err
	}
	for i := range spools {
		if GetSpoolExtraString(&spools[i], ExtraFieldTag) == trayUUID {
			return &spools[i], nil
		}
	}
	return nil, nil
}

// FindSpoolByActiveTray finds a spool assigned to a tray unique_id or entity_id.
func (c *Client) FindSpoolByActiveTray(trayID string, trayUniqueID string) (*Spool, error) {
	spools, err := c.GetAllSpools()
	if err != nil {
		return nil, err
	}
	for i := range spools {
		if spools[i].Extra == nil {
			continue
		}
		raw, ok := spools[i].Extra[ExtraFieldActiveTray]
		if !ok {
			continue
		}
		stored := GetSpoolExtraString(&spools[i], ExtraFieldActiveTray)
		if activeTrayMatches(raw, stored, trayID, trayUniqueID) {
			return &spools[i], nil
		}
	}
	return nil, nil
}

// UseSpoolWeight deducts weight from a spool via PUT /use.
func (c *Client) UseSpoolWeight(spoolID int, grams float64) error {
	payload, err := json.Marshal(map[string]float64{"use_weight": grams})
	if err != nil {
		return err
	}
	req, err := http.NewRequest("PUT", fmt.Sprintf("%s/api/v1/spool/%d/use", c.baseURL, spoolID), bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.addAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error using spool weight: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return c.handleAPIError(resp)
	}
	return nil
}

func (c *Client) patchSpoolExtra(spoolID int, extra map[string]string) error {
	payload, err := json.Marshal(map[string]interface{}{"extra": extra})
	if err != nil {
		return err
	}
	req, err := http.NewRequest("PATCH", fmt.Sprintf("%s/api/v1/spool/%d", c.baseURL, spoolID), bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.addAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error patching spool extra: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return c.handleAPIError(resp)
	}
	return nil
}
