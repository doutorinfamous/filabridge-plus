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

// SyncTrays updates the local tray cache for a registered Bambu printer.
func SyncTrays(b *core.FilamentBridge, printerID string, printer Printer) error {
	b.Mutex.Lock()
	defer b.Mutex.Unlock()

	if _, err := b.DB.Exec("DELETE FROM bambu_trays WHERE printer_id = ?", printerID); err != nil {
		return err
	}

	configs, _ := b.GetAllPrinterConfigs()
	printerName := ""
	if cfg, ok := configs[printerID]; ok {
		printerName = cfg.Name
	}

	insert := func(tray Tray, isExternal bool) error {
		displayName := tray.DisplayName
		if displayName == "" && printerName != "" {
			displayName = FormatTrayDisplayName(printerName, tray.AMSNumber, tray.TrayNumber, isExternal)
		}
		_, err := b.DB.Exec(`
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

// GetTraysForPrinter returns cached trays for a printer.
func GetTraysForPrinter(b *core.FilamentBridge, printerID string) ([]Tray, error) {
	rows, err := b.DB.Query(`
		SELECT tray_unique_id, entity_id, ams_number, tray_number, display_name, is_external
		FROM bambu_trays WHERE printer_id = ? ORDER BY ams_number, tray_number
	`, printerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trays []Tray
	for rows.Next() {
		var tray Tray
		var isExternal int
		if err := rows.Scan(&tray.UniqueID, &tray.EntityID, &tray.AMSNumber, &tray.TrayNumber, &tray.DisplayName, &isExternal); err != nil {
			return nil, err
		}
		tray.IsExternal = isExternal == 1
		trays = append(trays, tray)
	}
	return trays, nil
}

// FindTrayByUniqueID looks up a tray by unique_id.
func FindTrayByUniqueID(b *core.FilamentBridge, trayUniqueID string) (*Tray, error) {
	var tray Tray
	var isExternal int
	err := b.DB.QueryRow(`
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

// FindTrayByEntityID looks up a tray by Home Assistant entity_id.
func FindTrayByEntityID(b *core.FilamentBridge, entityID string) (*Tray, error) {
	var tray Tray
	var isExternal int
	err := b.DB.QueryRow(`
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

// FindTrayPrinterID returns the printer_id owning a tray (empty when unknown).
func FindTrayPrinterID(b *core.FilamentBridge, trayUniqueID string) (string, error) {
	var printerID string
	err := b.DB.QueryRow(
		"SELECT printer_id FROM bambu_trays WHERE tray_unique_id = ?",
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
	var tray Tray
	var isExternal int
	err := b.DB.QueryRow(`
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
	_, err := b.DB.Exec("DELETE FROM bambu_trays WHERE printer_id = ?", printerID)
	return err
}
