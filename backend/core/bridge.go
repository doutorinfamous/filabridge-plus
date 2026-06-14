// Package core holds the FilamentBridge: SQLite persistence, configuration and
// brand-neutral filament accounting shared by the snapmaker and bambu packages.
package core

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"filabridge/spoolman"
)

// FilamentBridge manages the connection between printers and Spoolman.
type FilamentBridge struct {
	Config           *Config
	Spoolman         *spoolman.Client
	DB               *sql.DB
	WasPrinting      map[string]bool
	CurrentJobFile   map[string]string          // Store current job filename per printer
	ProcessingPrints map[string]bool            // Track prints being processed
	PendingUsage     map[string]map[int]float64 // Cached filament usage awaiting spool update
	Mutex            sync.RWMutex

	printErrors map[string]PrintError // Store print processing errors
	errorMutex  sync.RWMutex
}

// NewFilamentBridge creates a new FilamentBridge instance.
func NewFilamentBridge(config *Config) (*FilamentBridge, error) {
	bridge := &FilamentBridge{
		Config:           config,
		Spoolman:         spoolman.NewClient(DefaultSpoolmanURL, SpoolmanTimeout, "", ""), // Default URL and timeout, will be updated
		WasPrinting:      make(map[string]bool),
		CurrentJobFile:   make(map[string]string),
		ProcessingPrints: make(map[string]bool),
		PendingUsage:     make(map[string]map[int]float64),
		printErrors:      make(map[string]PrintError),
	}

	if err := bridge.initDatabase(); err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	if config != nil && config.SpoolmanURL != "" {
		bridge.Spoolman = spoolman.NewClient(config.SpoolmanURL, config.SpoolmanTimeout, config.SpoolmanUsername, config.SpoolmanPassword)
	}

	return bridge, nil
}

// initDatabase initializes the SQLite database.
func (b *FilamentBridge) initDatabase() error {
	dbFile := DefaultDBFileName
	if b.Config != nil && b.Config.DBFile != "" {
		dbFile = b.Config.DBFile
	}
	// Check for environment variable (path only, append filename)
	if envDBPath := os.Getenv("FILABRIDGE_DB_PATH"); envDBPath != "" {
		dbFile = filepath.Join(envDBPath, DefaultDBFileName)
	}

	db, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	b.DB = db

	createTables := []string{
		`CREATE TABLE IF NOT EXISTS configuration (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			description TEXT,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS printer_configs (
			printer_id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			model TEXT,
			ip_address TEXT NOT NULL,
			api_key TEXT,
			toolheads INTEGER DEFAULT 1,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS printer_slots (
			slot_id TEXT PRIMARY KEY,
			printer_id TEXT NOT NULL,
			slot_type TEXT NOT NULL,
			toolhead_id INTEGER,
			entity_id TEXT,
			ams_number INTEGER,
			tray_number INTEGER,
			display_name TEXT NOT NULL DEFAULT '',
			spool_id INTEGER,
			mapped_at TIMESTAMP,
			UNIQUE (printer_id, toolhead_id)
		)`,
		`CREATE TABLE IF NOT EXISTS print_jobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			printer_id TEXT NOT NULL,
			job_name TEXT NOT NULL DEFAULT '',
			started_at TIMESTAMP,
			finished_at TIMESTAMP,
			status TEXT NOT NULL DEFAULT 'printing'
		)`,
		`CREATE TABLE IF NOT EXISTS filament_usage (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id INTEGER,
			printer_id TEXT NOT NULL,
			toolhead_id INTEGER,
			tray_unique_id TEXT,
			spool_id INTEGER NOT NULL,
			grams REAL NOT NULL,
			recorded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS nfc_sessions (
			session_id TEXT PRIMARY KEY,
			spool_id INTEGER,
			pending_filament_id INTEGER,
			printer_name TEXT,
			toolhead_id INTEGER,
			location_name TEXT,
			is_printer_location BOOLEAN,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			expires_at TIMESTAMP
		)`,
	}

	for _, query := range createTables {
		if _, err := b.DB.Exec(query); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	if err := b.initializeDefaultConfig(); err != nil {
		return fmt.Errorf("failed to initialize default configuration: %w", err)
	}

	if err := b.migrateLegacyPrinterConfig(); err != nil {
		log.Printf("Warning: Failed to migrate legacy printer configuration: %v", err)
	}

	if err := b.migrateBambuSchema(); err != nil {
		log.Printf("Warning: Failed to migrate Bambu schema: %v", err)
	}

	// Rebuild legacy tables (printer_name keys, toolhead_names, print_history,
	// processed_jobs) into the printer_id based schema.
	if err := b.migrateSchemaV2(); err != nil {
		return fmt.Errorf("failed to migrate database schema: %w", err)
	}

	// Merge toolhead_mappings + bambu_trays into the unified printer_slots table.
	if err := b.migrateSchemaV3(); err != nil {
		return fmt.Errorf("failed to migrate database schema to printer_slots: %w", err)
	}

	if err := b.migrateNFCSessionsSchema(); err != nil {
		return fmt.Errorf("failed to migrate NFC sessions schema: %w", err)
	}

	// Migrate existing FilaBridge locations to Spoolman
	if err := b.migrateLocationsToSpoolman(); err != nil {
		log.Printf("Warning: Failed to migrate locations to Spoolman: %v", err)
	}

	// Create Spoolman locations for existing toolhead mappings
	if err := b.migrateToolheadMappingsToSpoolman(); err != nil {
		log.Printf("Warning: Failed to migrate toolhead mappings to Spoolman: %v", err)
	}

	return nil
}

func (b *FilamentBridge) migrateNFCSessionsSchema() error {
	_, err := b.DB.Exec("ALTER TABLE nfc_sessions ADD COLUMN pending_filament_id INTEGER")
	if err != nil && !strings.Contains(err.Error(), "duplicate column") {
		return fmt.Errorf("failed to add pending_filament_id column: %w", err)
	}
	return nil
}

// migrateBambuSchema adds Bambu-specific columns and tables.
// The schema lives in core so all migrations run from one place; the Bambu
// business logic that uses these tables lives in the bambu package.
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
		_, err := b.DB.Exec(fmt.Sprintf("ALTER TABLE printer_configs ADD COLUMN %s %s", col.name, col.colType))
		if err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("failed to add column %s: %w", col.name, err)
		}
	}

	// HA package slugs must be lowercase — normalize any legacy uppercase prefixes
	if _, err := b.DB.Exec(`
		UPDATE printer_configs
		SET ha_prefix = lower(ha_prefix)
		WHERE driver = ? AND ha_prefix != lower(ha_prefix)
	`, DriverBambuHA); err != nil {
		return fmt.Errorf("failed to normalize bambu ha_prefix values: %w", err)
	}

	return nil
}

// migrateLegacyPrinterConfig copies legacy PrusaLink config keys to Moonraker keys.
func (b *FilamentBridge) migrateLegacyPrinterConfig() error {
	legacyMappings := map[string]string{
		ConfigKeyLegacyPrinterTimeout:             ConfigKeyPrinterTimeout,
		ConfigKeyLegacyPrinterFileDownloadTimeout: ConfigKeyPrinterFileDownloadTimeout,
		ConfigKeyLegacyAPIKey:                     ConfigKeyAPIKey,
	}

	for legacyKey, newKey := range legacyMappings {
		legacyValue, err := b.GetConfigValue(legacyKey)
		if err != nil {
			continue
		}
		if legacyValue == "" {
			continue
		}

		currentValue, err := b.GetConfigValue(newKey)
		if err == nil && currentValue != "" {
			continue
		}

		if err := b.SetConfigValue(newKey, legacyValue); err != nil {
			return fmt.Errorf("failed to migrate %s to %s: %w", legacyKey, newKey, err)
		}
		log.Printf("Migration: copied legacy config key %s to %s", legacyKey, newKey)
	}

	return nil
}

// migrateLocationsToSpoolman migrates existing FilaBridge locations to Spoolman.
func (b *FilamentBridge) migrateLocationsToSpoolman() error {
	rows, err := b.DB.Query("SELECT name, type, printer_name, toolhead_id FROM fb_locations")
	if err != nil {
		// Table doesn't exist or is empty, nothing to migrate
		return nil
	}
	defer rows.Close()

	migratedCount := 0
	for rows.Next() {
		var name, locationType, printerName sql.NullString
		var toolheadID sql.NullInt64

		if err := rows.Scan(&name, &locationType, &printerName, &toolheadID); err != nil {
			log.Printf("Warning: Failed to scan location row during migration: %v", err)
			continue
		}

		if !name.Valid || name.String == "" {
			continue
		}

		locationName := name.String

		if b.isVirtualPrinterToolheadLocation(locationName) {
			log.Printf("Migration: Skipping virtual printer toolhead location '%s'", locationName)
			continue
		}

		existingLocation, err := b.Spoolman.FindLocationByName(locationName)
		if err != nil {
			log.Printf("Warning: Failed to check if location '%s' exists in Spoolman: %v", locationName, err)
			continue
		}

		if existingLocation == nil {
			log.Printf("Migration: Location '%s' does not exist in Spoolman. It will be created when referenced in a spool, or can be created manually in Spoolman UI.", locationName)
		} else {
			migratedCount++
			log.Printf("Migration: Location '%s' already exists in Spoolman", locationName)
		}
	}

	if migratedCount > 0 {
		log.Printf("Migration: Successfully migrated %d location(s) from FilaBridge to Spoolman", migratedCount)
	}

	return nil
}

// migrateToolheadMappingsToSpoolman creates Spoolman locations for existing toolhead mappings.
func (b *FilamentBridge) migrateToolheadMappingsToSpoolman() error {
	printerConfigs, err := b.GetAllPrinterConfigs()
	if err != nil {
		return fmt.Errorf("failed to get printer configs: %w", err)
	}

	allMappings, err := b.GetAllToolheadMappings()
	if err != nil {
		return fmt.Errorf("failed to get toolhead mappings: %w", err)
	}

	createdCount := 0
	for printerID, printerMappings := range allMappings {
		config, exists := printerConfigs[printerID]
		if !exists {
			log.Printf("Migration: Could not find printer config for printer ID '%s', skipping", printerID)
			continue
		}
		printerName := config.Name

		for toolheadID, mapping := range printerMappings {
			displayName := mapping.DisplayName
			if displayName == "" {
				displayName = DefaultToolheadDisplayName(toolheadID)
			}

			locationName := fmt.Sprintf("%s - %s", printerName, displayName)

			existingLocation, err := b.Spoolman.FindLocationByName(locationName)
			if err != nil {
				log.Printf("Warning: Failed to check if toolhead location '%s' exists in Spoolman: %v", locationName, err)
				continue
			}

			if existingLocation == nil {
				log.Printf("Migration: Toolhead location '%s' does not exist in Spoolman. It will be created when a spool is assigned to this toolhead.", locationName)
			} else {
				createdCount++
				log.Printf("Migration: Toolhead location '%s' already exists in Spoolman", locationName)
			}
		}
	}

	if createdCount > 0 {
		log.Printf("Migration: Successfully created %d toolhead location(s) in Spoolman", createdCount)
	}

	return nil
}

// initializeDefaultConfig sets up default configuration values.
func (b *FilamentBridge) initializeDefaultConfig() error {
	defaultConfigs := map[string]string{
		ConfigKeyPrinterIPs:                      "", // Comma-separated list of printer IP addresses
		ConfigKeyAPIKey:                          "", // Optional Moonraker API key for authentication
		ConfigKeySpoolmanURL:                     DefaultSpoolmanURL,
		ConfigKeySpoolmanUsername:                "", // Spoolman basic auth username (optional)
		ConfigKeySpoolmanPassword:                "", // Spoolman basic auth password (optional)
		ConfigKeyPollInterval:                    fmt.Sprintf("%d", DefaultPollInterval),
		ConfigKeyWebPort:                         DefaultWebPort,
		ConfigKeyPrinterTimeout:                  fmt.Sprintf("%d", PrinterTimeout),
		ConfigKeyPrinterFileDownloadTimeout:      fmt.Sprintf("%d", PrinterFileDownloadTimeout),
		ConfigKeySpoolmanTimeout:                 fmt.Sprintf("%d", SpoolmanTimeout),
		ConfigKeyAutoAssignPreviousSpoolEnabled:  "false", // Enable auto-assignment of previous spool to default location
		ConfigKeyAutoAssignPreviousSpoolLocation: "",      // Default location name for auto-assigned previous spools
	}

	var totalCount int
	err := b.DB.QueryRow("SELECT COUNT(*) FROM configuration").Scan(&totalCount)
	if err != nil {
		return fmt.Errorf("failed to check config existence: %w", err)
	}

	// Only insert defaults if this is a fresh installation
	if totalCount == 0 {
		for key, value := range defaultConfigs {
			_, err := b.DB.Exec(
				"INSERT INTO configuration (key, value, description) VALUES (?, ?, ?)",
				key, value, getConfigDescription(key),
			)
			if err != nil {
				return fmt.Errorf("failed to insert default config %s: %w", key, err)
			}
		}
	}

	return nil
}

// getConfigDescription returns a description for a configuration key.
func getConfigDescription(key string) string {
	descriptions := map[string]string{
		ConfigKeyPrinterIPs:                      "Comma-separated list of printer IP addresses for Moonraker",
		ConfigKeyAPIKey:                          "Optional Moonraker API key for authentication",
		ConfigKeySpoolmanURL:                     "URL of Spoolman instance",
		ConfigKeySpoolmanUsername:                "Spoolman basic auth username (optional, leave empty if not using basic auth)",
		ConfigKeySpoolmanPassword:                "Spoolman basic auth password (optional, leave empty if not using basic auth)",
		ConfigKeyPollInterval:                    "Polling interval in seconds",
		ConfigKeyWebPort:                         "Port for web interface",
		ConfigKeyPrinterTimeout:                  "Printer Moonraker API timeout in seconds",
		ConfigKeyPrinterFileDownloadTimeout:      "Printer file download timeout in seconds",
		ConfigKeySpoolmanTimeout:                 "Spoolman API timeout in seconds",
		ConfigKeyAutoAssignPreviousSpoolEnabled:  "Enable automatic assignment of previous spool to default location when assigning new spool to toolhead",
		ConfigKeyAutoAssignPreviousSpoolLocation: "Default location name where previous spools will be automatically assigned (must exist as a location)",
		ConfigKeyHAURL:                           "Home Assistant URL (e.g. http://192.168.1.10:8123)",
		ConfigKeyHAToken:                         "Home Assistant Long-Lived Access Token",
		ConfigKeyFilabridgePublicURL:             "Public URL for FilaBridge webhooks (reachable from HA)",
	}
	if desc, exists := descriptions[key]; exists {
		return desc
	}
	return "Configuration value"
}

// GetConfigValue gets a configuration value from the database.
func (b *FilamentBridge) GetConfigValue(key string) (string, error) {
	var value string
	err := b.DB.QueryRow("SELECT value FROM configuration WHERE key = ?", key).Scan(&value)
	if err != nil {
		return "", fmt.Errorf("failed to get config value for %s: %w", key, err)
	}
	return value, nil
}

// SetConfigValue sets a configuration value in the database.
func (b *FilamentBridge) SetConfigValue(key, value string) error {
	_, err := b.DB.Exec(
		"INSERT OR REPLACE INTO configuration (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)",
		key, value,
	)
	if err != nil {
		return fmt.Errorf("failed to set config value for %s: %w", key, err)
	}
	return nil
}

// GetAllConfig gets all configuration values.
func (b *FilamentBridge) GetAllConfig() (map[string]string, error) {
	rows, err := b.DB.Query("SELECT key, value FROM configuration")
	if err != nil {
		return nil, fmt.Errorf("failed to get all config: %w", err)
	}
	defer rows.Close()

	config := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("failed to scan config row: %w", err)
		}
		config[key] = value
	}

	return config, nil
}

// GetAutoAssignPreviousSpoolEnabled gets whether auto-assignment of previous spool is enabled.
func (b *FilamentBridge) GetAutoAssignPreviousSpoolEnabled() (bool, error) {
	value, err := b.GetConfigValue(ConfigKeyAutoAssignPreviousSpoolEnabled)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return value == "true", nil
}

// SetAutoAssignPreviousSpoolEnabled sets whether auto-assignment of previous spool is enabled.
func (b *FilamentBridge) SetAutoAssignPreviousSpoolEnabled(enabled bool) error {
	value := "false"
	if enabled {
		value = "true"
	}
	return b.SetConfigValue(ConfigKeyAutoAssignPreviousSpoolEnabled, value)
}

// GetAutoAssignPreviousSpoolLocation gets the default location name for auto-assigned previous spools.
func (b *FilamentBridge) GetAutoAssignPreviousSpoolLocation() (string, error) {
	value, err := b.GetConfigValue(ConfigKeyAutoAssignPreviousSpoolLocation)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return value, nil
}

// SetAutoAssignPreviousSpoolLocation sets the default location name for auto-assigned previous spools.
func (b *FilamentBridge) SetAutoAssignPreviousSpoolLocation(location string) error {
	return b.SetConfigValue(ConfigKeyAutoAssignPreviousSpoolLocation, location)
}

// GetAllPrinterConfigs gets all printer configurations.
func (b *FilamentBridge) GetAllPrinterConfigs() (map[string]PrinterConfig, error) {
	rows, err := b.DB.Query("SELECT printer_id, name, model, COALESCE(driver, 'moonraker'), ip_address, api_key, toolheads, COALESCE(ha_prefix, ''), COALESCE(ha_device_id, '') FROM printer_configs")
	if err != nil {
		return nil, fmt.Errorf("failed to get printer configs: %w", err)
	}
	defer rows.Close()

	configs := make(map[string]PrinterConfig)
	for rows.Next() {
		var printerID, name, model, driver, ipAddress, apiKey, haPrefix, haDeviceID string
		var toolheads int
		if err := rows.Scan(&printerID, &name, &model, &driver, &ipAddress, &apiKey, &toolheads, &haPrefix, &haDeviceID); err != nil {
			return nil, fmt.Errorf("failed to scan printer config row: %w", err)
		}
		configs[printerID] = PrinterConfig{
			Name:       name,
			Model:      model,
			Driver:     driver,
			IPAddress:  ipAddress,
			APIKey:     apiKey,
			Toolheads:  toolheads,
			HAPrefix:   haPrefix,
			HADeviceID: haDeviceID,
		}
	}

	return configs, nil
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

// FindPrinterIDByName resolves a printer display name to its printer_id.
// Returns an empty string when no printer matches.
func (b *FilamentBridge) FindPrinterIDByName(name string) (string, error) {
	var printerID string
	err := b.DB.QueryRow("SELECT printer_id FROM printer_configs WHERE name = ? LIMIT 1", name).Scan(&printerID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to find printer by name: %w", err)
	}
	return printerID, nil
}

// ResolvePrinterDisplayName returns the configured display name for a printer ID,
// falling back to the ID itself when unknown.
func (b *FilamentBridge) ResolvePrinterDisplayName(printerID string) string {
	var name string
	err := b.DB.QueryRow("SELECT name FROM printer_configs WHERE printer_id = ?", printerID).Scan(&name)
	if err != nil || name == "" {
		return printerID
	}
	return name
}

// SavePrinterConfig saves a printer configuration.
func (b *FilamentBridge) SavePrinterConfig(printerID string, config PrinterConfig) error {
	b.Mutex.Lock()
	defer b.Mutex.Unlock()

	driver := config.Driver
	if driver == "" {
		driver = DriverMoonraker
	}
	_, err := b.DB.Exec(`
		INSERT OR REPLACE INTO printer_configs (printer_id, name, model, driver, ip_address, api_key, toolheads, ha_prefix, ha_device_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, printerID, config.Name, config.Model, driver, config.IPAddress, config.APIKey, config.Toolheads, config.HAPrefix, config.HADeviceID)
	if err != nil {
		return fmt.Errorf("failed to save printer config: %w", err)
	}
	return nil
}

// DeletePrinterConfig deletes a printer configuration.
func (b *FilamentBridge) DeletePrinterConfig(printerID string) error {
	b.Mutex.Lock()
	defer b.Mutex.Unlock()

	_, err := b.DB.Exec("DELETE FROM printer_configs WHERE printer_id = ?", printerID)
	if err != nil {
		return fmt.Errorf("failed to delete printer config: %w", err)
	}
	return nil
}

// GetConfigSnapshot returns a snapshot of the current config for safe iteration.
func (b *FilamentBridge) GetConfigSnapshot() *Config {
	b.Mutex.RLock()
	defer b.Mutex.RUnlock()

	if b.Config == nil {
		return nil
	}

	configCopy := &Config{
		SpoolmanURL:                b.Config.SpoolmanURL,
		PollInterval:               b.Config.PollInterval,
		DBFile:                     b.Config.DBFile,
		WebPort:                    b.Config.WebPort,
		PrinterTimeout:             b.Config.PrinterTimeout,
		PrinterFileDownloadTimeout: b.Config.PrinterFileDownloadTimeout,
		SpoolmanTimeout:            b.Config.SpoolmanTimeout,
		Printers:                   make(map[string]PrinterConfig),
	}

	for id, printer := range b.Config.Printers {
		configCopy.Printers[id] = printer
	}

	return configCopy
}

// ReloadConfig reloads the configuration from the database.
func (b *FilamentBridge) ReloadConfig() error {
	config, err := LoadConfig(b)
	if err != nil {
		return fmt.Errorf("failed to reload config: %w", err)
	}

	b.Mutex.Lock()
	b.Config = config
	if config.SpoolmanURL != "" {
		b.Spoolman = spoolman.NewClient(config.SpoolmanURL, config.SpoolmanTimeout, config.SpoolmanUsername, config.SpoolmanPassword)
	}
	b.Mutex.Unlock()

	return nil
}

// IsFirstRun checks if this is the first time the application is running.
func (b *FilamentBridge) IsFirstRun() (bool, error) {
	var count int
	err := b.DB.QueryRow("SELECT COUNT(*) FROM printer_configs").Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check first run status: %w", err)
	}

	return count == 0, nil
}

// UpdateConfig updates the bridge configuration.
func (b *FilamentBridge) UpdateConfig(config *Config) error {
	b.Mutex.Lock()
	defer b.Mutex.Unlock()

	b.Config = config
	b.Spoolman = spoolman.NewClient(config.SpoolmanURL, config.SpoolmanTimeout, config.SpoolmanUsername, config.SpoolmanPassword)

	return nil
}

// ParseVirtualToolheadLocation returns printer and toolhead display names when name
// matches a virtual printer toolhead location (e.g., "PrinterName - Toolhead 1").
func (b *FilamentBridge) ParseVirtualToolheadLocation(name string) (printerName, toolheadDisplayName string, ok bool) {
	printerConfigs, err := b.GetAllPrinterConfigs()
	if err != nil {
		log.Printf("Warning: Could not get printer configurations to check virtual location: %v", err)
		return "", "", false
	}

	for printerID, printerConfig := range printerConfigs {
		toolheadNames, err := b.GetAllToolheadNames(printerID)
		if err != nil {
			log.Printf("Warning: Could not get toolhead names for printer %s: %v", printerID, err)
			toolheadNames = make(map[int]string)
		}

		for toolheadID := 0; toolheadID < printerConfig.Toolheads; toolheadID++ {
			displayName := DefaultToolheadDisplayName(toolheadID)
			if customName, exists := toolheadNames[toolheadID]; exists {
				displayName = customName
			}

			expectedName := fmt.Sprintf("%s - %s", printerConfig.Name, displayName)
			if name == expectedName {
				return printerConfig.Name, displayName, true
			}
		}
	}

	return "", "", false
}

// isVirtualPrinterToolheadLocation checks if a location name matches the pattern
// of a virtual printer toolhead location (e.g., "PrinterName - Toolhead 1" or "PrinterName - Black").
func (b *FilamentBridge) isVirtualPrinterToolheadLocation(name string) bool {
	_, _, ok := b.ParseVirtualToolheadLocation(name)
	return ok
}

// Close closes the database connection.
func (b *FilamentBridge) Close() error {
	if b.DB != nil {
		return b.DB.Close()
	}
	return nil
}

// PendingUsageKey builds the cache key for pending filament usage per printer/file.
func PendingUsageKey(printerID, filename string) string {
	return printerID + "|" + filename
}

// sanitizeErrorID replaces problematic characters in error IDs to make them URL-safe.
func sanitizeErrorID(s string) string {
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	return s
}

// GetPrintErrors returns all unacknowledged print errors.
func (b *FilamentBridge) GetPrintErrors() []PrintError {
	b.errorMutex.RLock()
	defer b.errorMutex.RUnlock()

	var errors []PrintError
	for _, err := range b.printErrors {
		if !err.Acknowledged {
			errors = append(errors, err)
		}
	}
	return errors
}

// AcknowledgePrintError marks a print error as acknowledged.
func (b *FilamentBridge) AcknowledgePrintError(errorID string) error {
	b.errorMutex.Lock()
	defer b.errorMutex.Unlock()

	if err, exists := b.printErrors[errorID]; exists {
		err.Acknowledged = true
		b.printErrors[errorID] = err
		return nil
	}
	return fmt.Errorf("print error not found: %s", errorID)
}

// ResolvePrintError resolves a pending print error by assigning usage to a spool
// or dismissing the entire print without recording filament usage.
func (b *FilamentBridge) ResolvePrintError(errorID, action string, spoolID int) error {
	b.errorMutex.RLock()
	pe, exists := b.printErrors[errorID]
	if !exists || pe.Acknowledged {
		b.errorMutex.RUnlock()
		return fmt.Errorf("print error not found: %s", errorID)
	}
	b.errorMutex.RUnlock()

	printerID := pe.PrinterID
	if printerID == "" {
		var err error
		printerID, err = b.FindPrinterIDByName(pe.PrinterName)
		if err != nil {
			return fmt.Errorf("failed to resolve printer: %w", err)
		}
		if printerID == "" {
			printerID = pe.PrinterName
		}
	}

	jobName := pe.Filename
	if jobName == "" {
		jobName = pe.JobName
	}

	switch action {
	case ResolveActionAssignSpool:
		if spoolID <= 0 {
			return fmt.Errorf("spool_id is required")
		}
		if pe.Grams <= 0 {
			return fmt.Errorf("cannot assign spool: no gram amount recorded for this error")
		}
		if pe.ToolheadID == nil {
			return fmt.Errorf("cannot assign spool: no toolhead recorded for this error")
		}

		if err := b.Spoolman.UpdateSpoolUsage(spoolID, pe.Grams); err != nil {
			return fmt.Errorf("failed to update spool in Spoolman: %w", err)
		}

		jobID, err := b.GetLatestPrintJobID(printerID, jobName)
		if err != nil {
			return err
		}
		if jobID == 0 {
			jobID, err = b.StartPrintJob(printerID, jobName)
			if err != nil {
				return fmt.Errorf("failed to resolve print job: %w", err)
			}
		}

		if err := b.LogToolheadUsage(jobID, printerID, *pe.ToolheadID, spoolID, pe.Grams); err != nil {
			log.Printf("Warning: failed to log toolhead usage during error resolution: %v", err)
		}

		b.acknowledgePrintErrorByID(errorID)
		if !b.hasPendingPrintErrors(printerID, jobName) {
			if err := b.FinishPrintJob(printerID, jobName, JobStatusCompleted); err != nil {
				return err
			}
		}
		return nil

	case ResolveActionDismiss:
		b.acknowledgeAllPrintErrors(printerID, jobName)
		return b.FinishPrintJob(printerID, jobName, JobStatusCompleted)

	default:
		return fmt.Errorf("unknown action: %s", action)
	}
}

func (b *FilamentBridge) acknowledgePrintErrorByID(errorID string) {
	b.errorMutex.Lock()
	defer b.errorMutex.Unlock()

	if pe, exists := b.printErrors[errorID]; exists {
		pe.Acknowledged = true
		b.printErrors[errorID] = pe
	}
}

func (b *FilamentBridge) acknowledgeAllPrintErrors(printerID, jobName string) {
	b.errorMutex.Lock()
	defer b.errorMutex.Unlock()

	for id, pe := range b.printErrors {
		if pe.Acknowledged {
			continue
		}
		if !printErrorMatchesJob(pe, printerID, jobName) {
			continue
		}
		pe.Acknowledged = true
		b.printErrors[id] = pe
	}
}

func (b *FilamentBridge) hasPendingPrintErrors(printerID, jobName string) bool {
	b.errorMutex.RLock()
	defer b.errorMutex.RUnlock()

	for _, pe := range b.printErrors {
		if pe.Acknowledged {
			continue
		}
		if printErrorMatchesJob(pe, printerID, jobName) {
			return true
		}
	}
	return false
}

func printErrorMatchesJob(pe PrintError, printerID, jobName string) bool {
	peJob := pe.Filename
	if peJob == "" {
		peJob = pe.JobName
	}
	if peJob != jobName {
		return false
	}
	if pe.PrinterID != "" {
		return pe.PrinterID == printerID
	}
	return pe.PrinterName == printerID
}

// AddPrintError adds a new print error.
func (b *FilamentBridge) AddPrintError(input PrintErrorInput) {
	printerName := input.PrinterName
	jobName := input.JobName
	if printerName == "" {
		printerName = input.PrinterID
	}

	b.errorMutex.Lock()
	defer b.errorMutex.Unlock()

	for _, existing := range b.printErrors {
		if existing.Acknowledged {
			continue
		}
		if existing.PrinterName != printerName && existing.PrinterID != input.PrinterID {
			continue
		}
		existingJob := existing.Filename
		if existingJob == "" {
			existingJob = existing.JobName
		}
		if existingJob != jobName || existing.Error != input.Error {
			continue
		}
		return
	}

	sanitizedPrinterName := sanitizeErrorID(printerName)
	sanitizedFilename := sanitizeErrorID(jobName)
	errorID := fmt.Sprintf("%s_%s_%d", sanitizedPrinterName, sanitizedFilename, time.Now().Unix())

	pe := PrintError{
		ID:           errorID,
		PrinterID:    input.PrinterID,
		PrinterName:  printerName,
		Filename:     jobName,
		JobName:      jobName,
		Grams:        input.Grams,
		Error:        input.Error,
		Timestamp:    time.Now(),
		Acknowledged: false,
	}
	if input.ToolheadID >= 0 {
		toolheadID := input.ToolheadID
		pe.ToolheadID = &toolheadID
	}

	b.printErrors[errorID] = pe

	log.Printf("⚠️  Print processing failed for %s (%s): %s - Manual resolution required",
		printerName, jobName, input.Error)
}
