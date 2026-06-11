package core

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
)

// DefaultToolheadDisplayName returns the user-facing default name for a toolhead (1-based).
func DefaultToolheadDisplayName(toolheadID int) string {
	return fmt.Sprintf("Toolhead %d", toolheadID+1)
}

// GetToolheadName gets the display name for a toolhead, or returns default "Toolhead {N}" (1-based).
func (b *FilamentBridge) GetToolheadName(printerID string, toolheadID int) (string, error) {
	b.Mutex.RLock()
	defer b.Mutex.RUnlock()

	var displayName string
	err := b.DB.QueryRow(
		"SELECT display_name FROM toolhead_mappings WHERE printer_id = ? AND toolhead_id = ?",
		printerID, toolheadID,
	).Scan(&displayName)

	if err == sql.ErrNoRows || (err == nil && displayName == "") {
		return DefaultToolheadDisplayName(toolheadID), nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get toolhead name: %w", err)
	}

	return displayName, nil
}

// SetToolheadName sets the display name for a toolhead.
func (b *FilamentBridge) SetToolheadName(printerID string, toolheadID int, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("toolhead name cannot be empty")
	}

	// Get printer config to find printer name (before acquiring lock)
	printerConfigs, err := b.GetAllPrinterConfigs()
	if err != nil {
		return fmt.Errorf("failed to get printer configs: %w", err)
	}

	printerConfig, exists := printerConfigs[printerID]
	if !exists {
		return fmt.Errorf("printer %s not found", printerID)
	}

	printerName := printerConfig.Name

	// Get old toolhead name to calculate old location name (before acquiring lock)
	var oldDisplayName string
	oldName, err := b.GetToolheadName(printerID, toolheadID)
	if err == nil {
		oldDisplayName = oldName
	} else {
		oldDisplayName = DefaultToolheadDisplayName(toolheadID)
	}

	oldLocationName := fmt.Sprintf("%s - %s", printerName, oldDisplayName)
	newLocationName := fmt.Sprintf("%s - %s", printerName, name)

	b.Mutex.Lock()
	_, err = b.DB.Exec(`
		INSERT INTO toolhead_mappings (printer_id, toolhead_id, display_name)
		VALUES (?, ?, ?)
		ON CONFLICT(printer_id, toolhead_id) DO UPDATE SET display_name = excluded.display_name
	`, printerID, toolheadID, name)
	b.Mutex.Unlock()

	if err != nil {
		return fmt.Errorf("failed to set toolhead name: %w", err)
	}

	// If location name changed, update Spoolman (outside of lock)
	if oldLocationName != newLocationName {
		spools, err := b.Spoolman.GetAllSpools()
		if err != nil {
			log.Printf("Warning: Failed to get spools from Spoolman to update location names: %v", err)
		} else {
			updatedCount := 0
			for _, spool := range spools {
				if spool.Location == oldLocationName {
					if err := b.Spoolman.UpdateSpoolLocation(spool.ID, newLocationName); err != nil {
						log.Printf("Warning: Failed to update spool %d location from '%s' to '%s': %v", spool.ID, oldLocationName, newLocationName, err)
					} else {
						updatedCount++
					}
				}
			}

			if _, err := b.Spoolman.GetOrCreateLocation(newLocationName); err != nil {
				log.Printf("Warning: Failed to create/verify location '%s' in Spoolman: %v", newLocationName, err)
			}

			if updatedCount > 0 {
				log.Printf("Updated %d spool(s) location from '%s' to '%s'", updatedCount, oldLocationName, newLocationName)
			}
		}
	}

	log.Printf("Set toolhead name for printer %s, toolhead %d: %s", printerID, toolheadID, name)
	return nil
}

// GetAllToolheadNames gets all toolhead display names for a printer.
func (b *FilamentBridge) GetAllToolheadNames(printerID string) (map[int]string, error) {
	b.Mutex.RLock()
	defer b.Mutex.RUnlock()

	rows, err := b.DB.Query(
		"SELECT toolhead_id, display_name FROM toolhead_mappings WHERE printer_id = ? AND display_name != '' ORDER BY toolhead_id",
		printerID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get toolhead names: %w", err)
	}
	defer rows.Close()

	names := make(map[int]string)
	for rows.Next() {
		var toolheadID int
		var displayName string
		if err := rows.Scan(&toolheadID, &displayName); err != nil {
			return nil, fmt.Errorf("failed to scan toolhead name row: %w", err)
		}
		names[toolheadID] = displayName
	}

	return names, nil
}

// EnsurePrinterToolheadLocationsInSpoolman registers empty toolhead locations in Spoolman settings.
// fromToolheadID is inclusive; toolheadCount is the total number of toolheads on the printer.
func (b *FilamentBridge) EnsurePrinterToolheadLocationsInSpoolman(printerID, printerName string, fromToolheadID, toolheadCount int) {
	for toolheadID := fromToolheadID; toolheadID < toolheadCount; toolheadID++ {
		displayName, err := b.GetToolheadName(printerID, toolheadID)
		if err != nil {
			displayName = DefaultToolheadDisplayName(toolheadID)
		}
		locationName := fmt.Sprintf("%s - %s", printerName, displayName)
		if err := b.Spoolman.EnsureConfiguredLocation(locationName); err != nil {
			log.Printf("Warning: Failed to ensure Spoolman location '%s': %v", locationName, err)
		}
	}
}
