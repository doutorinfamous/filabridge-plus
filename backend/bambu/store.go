package bambu

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"filabridge/core"
	"filabridge/homeassistant"
)

// NormalizePrefix lowercases the printer prefix for HA package slugs and entity IDs.
// HA requires package filenames like filabridge_03919c461204338 (all lowercase).
func NormalizePrefix(prefix string) string {
	return strings.ToLower(strings.TrimSpace(prefix))
}

// GetHAURL returns configured Home Assistant URL (empty if not set).
func GetHAURL(b *core.FilamentBridge) (string, error) {
	value, err := b.GetConfigValue(core.ConfigKeyHAURL)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(value), nil
}

// GetHAToken returns configured Home Assistant token (empty if not set).
func GetHAToken(b *core.FilamentBridge) (string, error) {
	value, err := b.GetConfigValue(core.ConfigKeyHAToken)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(value), nil
}

// GetFilabridgePublicURL returns the URL HA uses for webhooks.
func GetFilabridgePublicURL(b *core.FilamentBridge) (string, error) {
	url, err := b.GetConfigValue(core.ConfigKeyFilabridgePublicURL)
	if err != nil || url == "" {
		port, _ := b.GetConfigValue(core.ConfigKeyWebPort)
		if port == "" {
			port = core.DefaultWebPort
		}
		return fmt.Sprintf("http://localhost:%s", port), nil
	}
	return strings.TrimRight(url, "/"), nil
}

// NewHAClientFromConfig creates an HA client from saved bridge configuration.
func NewHAClientFromConfig(b *core.FilamentBridge) (*homeassistant.Client, error) {
	url, err := GetHAURL(b)
	if err != nil {
		return nil, err
	}
	token, err := GetHAToken(b)
	if err != nil {
		return nil, err
	}
	return NewHAClientFromCredentials(url, token)
}

// NewHAClientFromCredentials validates and builds an HA client.
func NewHAClientFromCredentials(url, token string) (*homeassistant.Client, error) {
	url = strings.TrimSpace(url)
	token = strings.TrimSpace(token)
	if url == "" {
		return nil, fmt.Errorf("home assistant URL not configured — enter the URL and click Save HA Settings (or Test Connection)")
	}
	if token == "" {
		return nil, fmt.Errorf("home assistant token not configured — enter a Long-Lived Access Token and click Save HA Settings (or Test Connection)")
	}
	return homeassistant.NewClient(url, token), nil
}

// SyncTrays updates the local tray slots for a registered Bambu printer.
// Existing spool assignments (printer_slots.spool_id) are preserved.
func SyncTrays(b *core.FilamentBridge, printerID string, printer Printer) error {
	b.Mutex.Lock()
	defer b.Mutex.Unlock()

	configs, _ := b.GetAllPrinterConfigs()
	printerName := ""
	if cfg, ok := configs[printerID]; ok {
		printerName = cfg.Name
	}

	seen := make([]string, 0)
	upsert := func(tray Tray, isExternal bool) error {
		displayName := tray.DisplayName
		if displayName == "" && printerName != "" {
			displayName = FormatTrayDisplayName(printerName, tray.AMSNumber, tray.TrayNumber, isExternal)
		}
		slotType := core.SlotTypeAMSTray
		if isExternal {
			slotType = core.SlotTypeExternal
		}
		seen = append(seen, tray.UniqueID)
		_, err := b.DB.Exec(`
			INSERT INTO printer_slots (slot_id, printer_id, slot_type, entity_id, ams_number, tray_number, display_name)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(slot_id) DO UPDATE SET
				printer_id = excluded.printer_id,
				slot_type = excluded.slot_type,
				entity_id = excluded.entity_id,
				ams_number = excluded.ams_number,
				tray_number = excluded.tray_number,
				display_name = excluded.display_name
		`, tray.UniqueID, printerID, slotType, tray.EntityID, tray.AMSNumber, tray.TrayNumber, displayName)
		return err
	}

	for _, ext := range printer.ExternalSpools {
		if err := upsert(ext, true); err != nil {
			return err
		}
	}
	for _, ams := range printer.AMSUnits {
		for _, tray := range ams.Trays {
			t := tray
			t.AMSNumber = ams.AMSNumber
			if err := upsert(t, false); err != nil {
				return err
			}
		}
	}

	// Drop tray slots that no longer exist on the printer.
	args := []interface{}{printerID, core.SlotTypeAMSTray, core.SlotTypeExternal}
	placeholders := ""
	for i, id := range seen {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args = append(args, id)
	}
	query := "DELETE FROM printer_slots WHERE printer_id = ? AND slot_type IN (?, ?)"
	if len(seen) > 0 {
		query += " AND slot_id NOT IN (" + placeholders + ")"
	}
	if _, err := b.DB.Exec(query, args...); err != nil {
		return err
	}
	return nil
}

const traySelectColumns = `slot_id, entity_id, ams_number, tray_number, display_name, slot_type, COALESCE(spool_id, 0)`

func scanTray(row interface{ Scan(...interface{}) error }) (Tray, error) {
	var tray Tray
	var slotType string
	var spoolID int
	err := row.Scan(&tray.UniqueID, &tray.EntityID, &tray.AMSNumber, &tray.TrayNumber, &tray.DisplayName, &slotType, &spoolID)
	if err != nil {
		return tray, err
	}
	tray.IsExternal = slotType == core.SlotTypeExternal
	if spoolID > 0 {
		tray.AssignedSpoolID = &spoolID
	}
	return tray, nil
}

// GetTraysForPrinter returns cached trays for a printer.
func GetTraysForPrinter(b *core.FilamentBridge, printerID string) ([]Tray, error) {
	rows, err := b.DB.Query(fmt.Sprintf(`
		SELECT %s FROM printer_slots
		WHERE printer_id = ? AND slot_type IN (?, ?)
		ORDER BY ams_number, tray_number
	`, traySelectColumns), printerID, core.SlotTypeAMSTray, core.SlotTypeExternal)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trays []Tray
	for rows.Next() {
		tray, err := scanTray(rows)
		if err != nil {
			return nil, err
		}
		trays = append(trays, tray)
	}
	return trays, nil
}

func findTray(b *core.FilamentBridge, where string, arg interface{}) (*Tray, error) {
	row := b.DB.QueryRow(fmt.Sprintf(`
		SELECT %s FROM printer_slots
		WHERE slot_type IN (?, ?) AND %s
	`, traySelectColumns, where), core.SlotTypeAMSTray, core.SlotTypeExternal, arg)
	tray, err := scanTray(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &tray, nil
}

// FindTrayByUniqueID looks up a tray by unique_id.
func FindTrayByUniqueID(b *core.FilamentBridge, trayUniqueID string) (*Tray, error) {
	return findTray(b, "slot_id = ?", trayUniqueID)
}

// FindTrayByEntityID looks up a tray by Home Assistant entity_id.
func FindTrayByEntityID(b *core.FilamentBridge, entityID string) (*Tray, error) {
	return findTray(b, "entity_id = ?", entityID)
}

// FindTrayPrinterID returns the printer_id owning a tray (empty when unknown).
func FindTrayPrinterID(b *core.FilamentBridge, trayUniqueID string) (string, error) {
	var printerID string
	err := b.DB.QueryRow(
		"SELECT printer_id FROM printer_slots WHERE slot_id = ?",
		trayUniqueID,
	).Scan(&printerID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return printerID, nil
}

// FindPrinterIDByPrefix resolves a Bambu HA prefix to the printer_id (empty when unknown).
func FindPrinterIDByPrefix(b *core.FilamentBridge, prefix string) (string, error) {
	prefix = NormalizePrefix(prefix)
	if prefix == "" {
		return "", nil
	}
	var printerID string
	err := b.DB.QueryRow(
		"SELECT printer_id FROM printer_configs WHERE driver = ? AND lower(ha_prefix) = ? LIMIT 1",
		core.DriverBambuHA, prefix,
	).Scan(&printerID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return printerID, nil
}

// FindTrayByDisplayName resolves a display name to tray unique_id.
func FindTrayByDisplayName(b *core.FilamentBridge, displayName string) (*Tray, error) {
	return findTray(b, "display_name = ?", displayName)
}

// RegisterPrinter saves a discovered Bambu printer in FilaBridge.
func RegisterPrinter(b *core.FilamentBridge, printer Printer) (string, error) {
	prefix := NormalizePrefix(printer.Prefix)
	printer.Prefix = prefix
	printerID := "bambu_" + prefix
	cfg := core.PrinterConfig{
		Name:       printer.Name,
		Model:      ModelBambuHA,
		Driver:     core.DriverBambuHA,
		HAPrefix:   prefix,
		HADeviceID: printer.DeviceID,
		Toolheads:  0,
	}
	if err := b.SavePrinterConfig(printerID, cfg); err != nil {
		return "", err
	}
	if err := SyncTrays(b, printerID, printer); err != nil {
		return "", err
	}
	return printerID, nil
}

// RemovePrinter removes a Bambu printer from FilaBridge (not from HA).
func RemovePrinter(b *core.FilamentBridge, printerID string) error {
	if err := b.DeletePrinterConfig(printerID); err != nil {
		return err
	}
	_, err := b.DB.Exec("DELETE FROM printer_slots WHERE printer_id = ?", printerID)
	return err
}
