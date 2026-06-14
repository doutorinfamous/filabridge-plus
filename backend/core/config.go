package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// PrinterConfig represents configuration for a single printer.
type PrinterConfig struct {
	Name       string `json:"name"`
	Model      string `json:"model"`
	Driver     string `json:"driver,omitempty"`
	IPAddress  string `json:"ip_address"`
	APIKey     string `json:"api_key,omitempty"`
	Toolheads  int    `json:"toolheads"`
	HAPrefix   string `json:"ha_prefix,omitempty"`
	HADeviceID string `json:"ha_device_id,omitempty"`
}

// Config holds all configuration for the application.
type Config struct {
	SpoolmanURL                string
	SpoolmanUsername           string
	SpoolmanPassword           string
	PollInterval               time.Duration
	LocationSyncInterval       time.Duration
	DBFile                     string
	WebPort                    string
	PrinterTimeout             int
	PrinterFileDownloadTimeout int
	SpoolmanTimeout            int
	Printers                   map[string]PrinterConfig // Key is printer ID, value is printer config
}

// LoadConfig loads configuration from database.
func LoadConfig(bridge *FilamentBridge) (*Config, error) {
	configValues, err := bridge.GetAllConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config from database: %w", err)
	}

	pollInterval := DefaultPollInterval
	if pollStr, exists := configValues[ConfigKeyPollInterval]; exists {
		if parsed, err := strconv.Atoi(pollStr); err == nil {
			pollInterval = parsed
		}
	}

	locationSyncInterval := DefaultLocationSyncInterval
	if syncStr, exists := configValues[ConfigKeyLocationSyncInterval]; exists {
		if parsed, err := strconv.Atoi(syncStr); err == nil {
			locationSyncInterval = parsed
		}
	}

	// Parse timeout values (with legacy PrusaLink key fallback)
	printerTimeout := PrinterTimeout
	if timeoutStr, exists := configValues[ConfigKeyPrinterTimeout]; exists {
		if parsed, err := strconv.Atoi(timeoutStr); err == nil {
			printerTimeout = parsed
		}
	} else if timeoutStr, exists := configValues[ConfigKeyLegacyPrinterTimeout]; exists {
		if parsed, err := strconv.Atoi(timeoutStr); err == nil {
			printerTimeout = parsed
		}
	}

	printerFileDownloadTimeout := PrinterFileDownloadTimeout
	if timeoutStr, exists := configValues[ConfigKeyPrinterFileDownloadTimeout]; exists {
		if parsed, err := strconv.Atoi(timeoutStr); err == nil {
			printerFileDownloadTimeout = parsed
		}
	} else if timeoutStr, exists := configValues[ConfigKeyLegacyPrinterFileDownloadTimeout]; exists {
		if parsed, err := strconv.Atoi(timeoutStr); err == nil {
			printerFileDownloadTimeout = parsed
		}
	}

	spoolmanTimeout := SpoolmanTimeout
	if timeoutStr, exists := configValues[ConfigKeySpoolmanTimeout]; exists {
		if parsed, err := strconv.Atoi(timeoutStr); err == nil {
			spoolmanTimeout = parsed
		}
	}

	config := &Config{
		SpoolmanURL:                configValues[ConfigKeySpoolmanURL],
		SpoolmanUsername:           configValues[ConfigKeySpoolmanUsername],
		SpoolmanPassword:           configValues[ConfigKeySpoolmanPassword],
		PollInterval:               time.Duration(pollInterval) * time.Second,
		LocationSyncInterval:       time.Duration(locationSyncInterval) * time.Minute,
		DBFile:                     getDBFilePath(),
		WebPort:                    configValues[ConfigKeyWebPort],
		PrinterTimeout:             printerTimeout,
		PrinterFileDownloadTimeout: printerFileDownloadTimeout,
		SpoolmanTimeout:            spoolmanTimeout,
		Printers:                   make(map[string]PrinterConfig),
	}

	printerConfigs, err := bridge.GetAllPrinterConfigs()
	if err != nil {
		fmt.Printf("Error loading printer configs: %v\n", err)
		config.Printers["no_printers"] = PrinterConfig{
			Name:      "No Printers Configured",
			Model:     "Unknown",
			IPAddress: "",
			APIKey:    "",
			Toolheads: 0,
		}
		return config, nil
	}

	for printerID, printerConfig := range printerConfigs {
		config.Printers[printerID] = PrinterConfig{
			Name:       printerConfig.Name,
			Model:      printerConfig.Model,
			Driver:     printerConfig.Driver,
			IPAddress:  printerConfig.IPAddress,
			APIKey:     printerConfig.APIKey,
			Toolheads:  printerConfig.Toolheads,
			HAPrefix:   printerConfig.HAPrefix,
			HADeviceID: printerConfig.HADeviceID,
		}
	}

	if len(config.Printers) == 0 {
		config.Printers["no_printers"] = PrinterConfig{
			Name:      "No Printers Configured",
			Model:     "Unknown",
			IPAddress: "",
			APIKey:    "",
			Toolheads: 0,
		}
	}

	return config, nil
}

// ResolvePrinterName resolves printer name from config, with fallback to IP-based name.
func ResolvePrinterName(config PrinterConfig) string {
	if config.Name != "" {
		return config.Name
	}
	return fmt.Sprintf("Printer_%s", config.IPAddress)
}

// getDBFilePath returns the database file path, checking environment variable first.
func getDBFilePath() string {
	if dbPath := os.Getenv("FILABRIDGE_DB_PATH"); dbPath != "" {
		return filepath.Join(dbPath, DefaultDBFileName)
	}
	return DefaultDBFileName
}
