package main

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// migrateBambuSchema adds Bambu-specific columns and tables.
func (b *FilamentBridge) migrateBambuSchema() error {
	columns := []struct {
		name    string
		colType string
	}{
		{"driver", "TEXT DEFAULT 'moonraker'"},
		{"ha_prefix", "TEXT DEFAULT ''"},
		{"ha_device_id", "TEXT DEFAULT ''"},
	}
	for _, col := range columns {
		_, err := b.db.Exec(fmt.Sprintf("ALTER TABLE printer_configs ADD COLUMN %s %s", col.name, col.colType))
		if err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("failed to add column %s: %w", col.name, err)
		}
	}

	_, err := b.db.Exec(`
		CREATE TABLE IF NOT EXISTS bambu_trays (
			printer_id TEXT NOT NULL,
			tray_unique_id TEXT PRIMARY KEY,
			entity_id TEXT NOT NULL,
			ams_number INTEGER NOT NULL DEFAULT 0,
			tray_number INTEGER NOT NULL DEFAULT 0,
			display_name TEXT NOT NULL,
			is_external INTEGER NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create bambu_trays table: %w", err)
	}

	// HA package slugs must be lowercase — normalize any legacy uppercase prefixes
	if _, err := b.db.Exec(`
		UPDATE printer_configs
		SET ha_prefix = lower(ha_prefix)
		WHERE driver = ? AND ha_prefix != lower(ha_prefix)
	`, DriverBambuHA); err != nil {
		return fmt.Errorf("failed to normalize bambu ha_prefix values: %w", err)
	}

	return nil
}

// normalizeBambuHAPrefix lowercases the printer prefix for HA package slugs and entity IDs.
// HA requires package filenames like filabridge_03919c461204338 (all lowercase).
func normalizeBambuHAPrefix(prefix string) string {
	return strings.ToLower(strings.TrimSpace(prefix))
}

// GetHAURL returns configured Home Assistant URL (empty if not set).
func (b *FilamentBridge) GetHAURL() (string, error) {
	value, err := b.GetConfigValue(ConfigKeyHAURL)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(value), nil
}

// GetHAToken returns configured Home Assistant token (empty if not set).
func (b *FilamentBridge) GetHAToken() (string, error) {
	value, err := b.GetConfigValue(ConfigKeyHAToken)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(value), nil
}

// GetFilabridgePublicURL returns the URL HA uses for webhooks.
func (b *FilamentBridge) GetFilabridgePublicURL() (string, error) {
	url, err := b.GetConfigValue(ConfigKeyFilabridgePublicURL)
	if err != nil || url == "" {
		port, _ := b.GetConfigValue(ConfigKeyWebPort)
		if port == "" {
			port = DefaultWebPort
		}
		return fmt.Sprintf("http://localhost:%s", port), nil
	}
	return strings.TrimRight(url, "/"), nil
}

// NewHAClientFromConfig creates an HA client from saved bridge configuration.
func (b *FilamentBridge) NewHAClientFromConfig() (*HAClient, error) {
	url, err := b.GetHAURL()
	if err != nil {
		return nil, err
	}
	token, err := b.GetHAToken()
	if err != nil {
		return nil, err
	}
	return newHAClientFromCredentials(url, token)
}

func newHAClientFromCredentials(url, token string) (*HAClient, error) {
	url = strings.TrimSpace(url)
	token = strings.TrimSpace(token)
	if url == "" {
		return nil, fmt.Errorf("home assistant URL not configured — enter the URL and click Save HA Settings (or Test Connection)")
	}
	if token == "" {
		return nil, fmt.Errorf("home assistant token not configured — enter a Long-Lived Access Token and click Save HA Settings (or Test Connection)")
	}
	return NewHAClient(url, token), nil
}

// GetMoonrakerPrinterConfigs returns only Moonraker driver printers.
func (b *FilamentBridge) GetMoonrakerPrinterConfigs() (map[string]PrinterConfig, error) {
	all, err := b.GetAllPrinterConfigs()
	if err != nil {
		return nil, err
	}
	result := make(map[string]PrinterConfig)
	for id, cfg := range all {
		if cfg.Driver == "" || cfg.Driver == DriverMoonraker {
			result[id] = cfg
		}
	}
	return result, nil
}

// GetBambuPrinterConfigs returns only Bambu HA driver printers.
func (b *FilamentBridge) GetBambuPrinterConfigs() (map[string]PrinterConfig, error) {
	all, err := b.GetAllPrinterConfigs()
	if err != nil {
		return nil, err
	}
	result := make(map[string]PrinterConfig)
	for id, cfg := range all {
		if cfg.Driver == DriverBambuHA {
			result[id] = cfg
		}
	}
	return result, nil
}

// SyncBambuTrays updates the local tray cache for a registered Bambu printer.
func (b *FilamentBridge) SyncBambuTrays(printerID string, printer BambuPrinter) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if _, err := b.db.Exec("DELETE FROM bambu_trays WHERE printer_id = ?", printerID); err != nil {
		return err
	}

	configs, _ := b.GetAllPrinterConfigs()
	printerName := ""
	if cfg, ok := configs[printerID]; ok {
		printerName = cfg.Name
	}

	insert := func(tray BambuTray, isExternal bool) error {
		displayName := tray.DisplayName
		if displayName == "" && printerName != "" {
			displayName = formatBambuTrayDisplayName(printerName, tray.AMSNumber, tray.TrayNumber, isExternal)
		}
		_, err := b.db.Exec(`
			INSERT INTO bambu_trays (printer_id, tray_unique_id, entity_id, ams_number, tray_number, display_name, is_external)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, printerID, tray.UniqueID, tray.EntityID, tray.AMSNumber, tray.TrayNumber, displayName, boolToInt(isExternal))
		return err
	}

	for _, ext := range printer.ExternalSpools {
		if err := insert(ext, true); err != nil {
			return err
		}
	}
	for _, ams := range printer.AMSUnits {
		for _, tray := range ams.Trays {
			t := tray
			t.AMSNumber = ams.AMSNumber
			if err := insert(t, false); err != nil {
				return err
			}
		}
	}
	return nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// GetBambuTraysForPrinter returns cached trays for a printer.
func (b *FilamentBridge) GetBambuTraysForPrinter(printerID string) ([]BambuTray, error) {
	rows, err := b.db.Query(`
		SELECT tray_unique_id, entity_id, ams_number, tray_number, display_name, is_external
		FROM bambu_trays WHERE printer_id = ? ORDER BY ams_number, tray_number
	`, printerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trays []BambuTray
	for rows.Next() {
		var tray BambuTray
		var isExternal int
		if err := rows.Scan(&tray.UniqueID, &tray.EntityID, &tray.AMSNumber, &tray.TrayNumber, &tray.DisplayName, &isExternal); err != nil {
			return nil, err
		}
		tray.IsExternal = isExternal == 1
		trays = append(trays, tray)
	}
	return trays, nil
}

// FindBambuTrayByUniqueID looks up a tray by unique_id.
func (b *FilamentBridge) FindBambuTrayByUniqueID(trayUniqueID string) (*BambuTray, error) {
	var tray BambuTray
	var isExternal int
	err := b.db.QueryRow(`
		SELECT tray_unique_id, entity_id, ams_number, tray_number, display_name, is_external
		FROM bambu_trays WHERE tray_unique_id = ?
	`, trayUniqueID).Scan(&tray.UniqueID, &tray.EntityID, &tray.AMSNumber, &tray.TrayNumber, &tray.DisplayName, &isExternal)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	tray.IsExternal = isExternal == 1
	return &tray, nil
}

// FindBambuTrayByEntityID looks up a tray by Home Assistant entity_id.
func (b *FilamentBridge) FindBambuTrayByEntityID(entityID string) (*BambuTray, error) {
	var tray BambuTray
	var isExternal int
	err := b.db.QueryRow(`
		SELECT tray_unique_id, entity_id, ams_number, tray_number, display_name, is_external
		FROM bambu_trays WHERE entity_id = ?
	`, entityID).Scan(&tray.UniqueID, &tray.EntityID, &tray.AMSNumber, &tray.TrayNumber, &tray.DisplayName, &isExternal)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	tray.IsExternal = isExternal == 1
	return &tray, nil
}

// FindBambuTrayByDisplayName resolves a display name to tray unique_id.
func (b *FilamentBridge) FindBambuTrayByDisplayName(displayName string) (*BambuTray, error) {
	var tray BambuTray
	var isExternal int
	err := b.db.QueryRow(`
		SELECT tray_unique_id, entity_id, ams_number, tray_number, display_name, is_external
		FROM bambu_trays WHERE display_name = ?
	`, displayName).Scan(&tray.UniqueID, &tray.EntityID, &tray.AMSNumber, &tray.TrayNumber, &tray.DisplayName, &isExternal)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	tray.IsExternal = isExternal == 1
	return &tray, nil
}

// RegisterBambuPrinter saves a discovered Bambu printer in FilaBridge.
func (b *FilamentBridge) RegisterBambuPrinter(printer BambuPrinter) (string, error) {
	prefix := normalizeBambuHAPrefix(printer.Prefix)
	printer.Prefix = prefix
	printerID := "bambu_" + prefix
	cfg := PrinterConfig{
		Name:       printer.Name,
		Model:      ModelBambuHA,
		Driver:     DriverBambuHA,
		HAPrefix:   prefix,
		HADeviceID: printer.DeviceID,
		Toolheads:  0,
	}
	if err := b.SavePrinterConfig(printerID, cfg); err != nil {
		return "", err
	}
	if err := b.SyncBambuTrays(printerID, printer); err != nil {
		return "", err
	}
	return printerID, nil
}

// RemoveBambuPrinter removes a Bambu printer from FilaBridge (not from HA).
func (b *FilamentBridge) RemoveBambuPrinter(printerID string) error {
	if err := b.DeletePrinterConfig(printerID); err != nil {
		return err
	}
	_, err := b.db.Exec("DELETE FROM bambu_trays WHERE printer_id = ?", printerID)
	return err
}
