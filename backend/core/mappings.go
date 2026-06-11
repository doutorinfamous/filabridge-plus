package core

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// GetToolheadMapping gets the spool ID mapped to a specific toolhead.
func (b *FilamentBridge) GetToolheadMapping(printerID string, toolheadID int) (int, error) {
	b.Mutex.RLock()
	defer b.Mutex.RUnlock()

	return b.getToolheadMappingLocked(printerID, toolheadID)
}

// getToolheadMappingLocked must be called with b.Mutex already held (read or write).
func (b *FilamentBridge) getToolheadMappingLocked(printerID string, toolheadID int) (int, error) {
	var spoolID int
	err := b.DB.QueryRow(
		"SELECT COALESCE(spool_id, 0) FROM toolhead_mappings WHERE printer_id = ? AND toolhead_id = ?",
		printerID, toolheadID,
	).Scan(&spoolID)

	if err == sql.ErrNoRows {
		return 0, nil // No mapping found
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get toolhead mapping: %w", err)
	}

	return spoolID, nil
}

// SetToolheadMapping maps a spool to a specific toolhead, preserving any custom display name.
func (b *FilamentBridge) SetToolheadMapping(printerID string, toolheadID int, spoolID int) error {
	b.Mutex.Lock()

	// Get the previous spool ID before replacing it (for auto-assignment feature)
	previousSpoolID, err := b.getToolheadMappingLocked(printerID, toolheadID)
	if err != nil {
		b.Mutex.Unlock()
		return fmt.Errorf("failed to get previous spool mapping: %w", err)
	}

	if err := b.ensureSpoolNotAssignedElsewhereLocked(spoolID, ExcludeAssignment{
		PrinterID:  printerID,
		ToolheadID: toolheadID,
	}); err != nil {
		b.Mutex.Unlock()
		return err
	}

	_, err = b.DB.Exec(`
		INSERT INTO toolhead_mappings (printer_id, toolhead_id, spool_id, mapped_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(printer_id, toolhead_id) DO UPDATE SET spool_id = excluded.spool_id, mapped_at = excluded.mapped_at
	`, printerID, toolheadID, spoolID, time.Now())
	if err != nil {
		b.Mutex.Unlock()
		return fmt.Errorf("failed to set toolhead mapping: %w", err)
	}

	log.Printf("Mapped %s toolhead %d to spool %d", printerID, toolheadID, spoolID)

	// Check if auto-assign feature is enabled and we have a previous spool to assign
	enabled, err := b.GetAutoAssignPreviousSpoolEnabled()
	if err != nil {
		log.Printf("Warning: Failed to check auto-assign previous spool setting: %v", err)
		b.Mutex.Unlock()
		return nil // Don't fail the assignment if we can't check the setting
	}

	// Unlock before potentially calling AssignSpoolToLocation (which may need locks)
	b.Mutex.Unlock()

	if enabled && previousSpoolID > 0 && previousSpoolID != spoolID {
		locationName, err := b.GetAutoAssignPreviousSpoolLocation()
		if err != nil {
			log.Printf("Warning: Failed to get auto-assign previous spool location setting: %v", err)
			return nil
		}

		if locationName != "" {
			location, err := b.Spoolman.FindLocationByName(locationName)
			if err != nil || location == nil {
				log.Printf("Warning: Auto-assign previous spool location '%s' does not exist, skipping auto-assignment of spool %d", locationName, previousSpoolID)
				return nil
			}

			// Use isPrinterLocation = false since this is a storage location
			if err := b.AssignSpoolToLocation(previousSpoolID, "", 0, locationName, false); err != nil {
				log.Printf("Warning: Failed to auto-assign previous spool %d to location '%s': %v", previousSpoolID, locationName, err)
			} else {
				log.Printf("Auto-assigned previous spool %d to location '%s'", previousSpoolID, locationName)
			}
		}
	}

	return nil
}

// TryAutoAssignSpoolToDefaultStorage moves an unmapped spool to the configured storage location.
func (b *FilamentBridge) TryAutoAssignSpoolToDefaultStorage(spoolID int) {
	if spoolID <= 0 {
		return
	}

	enabled, err := b.GetAutoAssignPreviousSpoolEnabled()
	if err != nil {
		log.Printf("Warning: Failed to check auto-assign previous spool setting: %v", err)
		return
	}
	if !enabled {
		return
	}

	locationName, err := b.GetAutoAssignPreviousSpoolLocation()
	if err != nil {
		log.Printf("Warning: Failed to get auto-assign previous spool location setting: %v", err)
		return
	}
	if locationName == "" {
		return
	}

	location, err := b.Spoolman.FindLocationByName(locationName)
	if err != nil || location == nil {
		log.Printf("Warning: Auto-assign previous spool location '%s' does not exist, skipping auto-assignment of spool %d", locationName, spoolID)
		return
	}

	if err := b.AssignSpoolToLocation(spoolID, "", 0, locationName, false); err != nil {
		log.Printf("Warning: Failed to auto-assign unmapped spool %d to location '%s': %v", spoolID, locationName, err)
		return
	}
	log.Printf("Auto-assigned unmapped spool %d to location '%s'", spoolID, locationName)
}

// GetToolheadMappings gets all toolhead mappings for a printer (keyed by toolhead ID).
func (b *FilamentBridge) GetToolheadMappings(printerID string) (map[int]ToolheadMapping, error) {
	rows, err := b.DB.Query(
		"SELECT toolhead_id, COALESCE(spool_id, 0), display_name, mapped_at FROM toolhead_mappings WHERE printer_id = ?",
		printerID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	mappings := make(map[int]ToolheadMapping)
	for rows.Next() {
		var toolheadID, spoolID int
		var displayName string
		var mappedAt time.Time
		if err := rows.Scan(&toolheadID, &spoolID, &displayName, &mappedAt); err != nil {
			return nil, err
		}
		mappings[toolheadID] = ToolheadMapping{
			PrinterID:   printerID,
			ToolheadID:  toolheadID,
			SpoolID:     spoolID,
			MappedAt:    mappedAt,
			DisplayName: displayName,
		}
	}

	return mappings, nil
}

// GetAllToolheadMappings gets all toolhead mappings across all printers (keyed by printer ID).
func (b *FilamentBridge) GetAllToolheadMappings() (map[string]map[int]ToolheadMapping, error) {
	rows, err := b.DB.Query(
		"SELECT printer_id, toolhead_id, COALESCE(spool_id, 0), display_name, mapped_at FROM toolhead_mappings ORDER BY printer_id, toolhead_id",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	mappings := make(map[string]map[int]ToolheadMapping)
	for rows.Next() {
		var printerID, displayName string
		var toolheadID, spoolID int
		var mappedAt time.Time
		if err := rows.Scan(&printerID, &toolheadID, &spoolID, &displayName, &mappedAt); err != nil {
			return nil, err
		}

		if mappings[printerID] == nil {
			mappings[printerID] = make(map[int]ToolheadMapping)
		}

		mappings[printerID][toolheadID] = ToolheadMapping{
			PrinterID:   printerID,
			ToolheadID:  toolheadID,
			SpoolID:     spoolID,
			MappedAt:    mappedAt,
			DisplayName: displayName,
		}
	}

	return mappings, nil
}

// UnmapToolhead removes the spool from a toolhead, keeping its custom display name.
func (b *FilamentBridge) UnmapToolhead(printerID string, toolheadID int) error {
	b.Mutex.Lock()
	defer b.Mutex.Unlock()

	_, err := b.DB.Exec(
		"UPDATE toolhead_mappings SET spool_id = NULL, mapped_at = ? WHERE printer_id = ? AND toolhead_id = ?",
		time.Now(), printerID, toolheadID,
	)
	if err != nil {
		return fmt.Errorf("failed to unmap toolhead: %w", err)
	}

	log.Printf("Unmapped %s toolhead %d", printerID, toolheadID)
	return nil
}
