package core

import (
	"database/sql"
	"fmt"
	"log"

	"filabridge/spoolman"
)

// ExcludeAssignment identifies a mapping context whose current spool stays selectable.
type ExcludeAssignment struct {
	PrinterID    string
	ToolheadID   int
	TrayUniqueID string
}

// GetAssignedSpoolIDs returns spool IDs in use across Moonraker toolheads and Bambu AMS trays.
func (b *FilamentBridge) GetAssignedSpoolIDs(exclude ExcludeAssignment) (map[int]bool, error) {
	assigned := make(map[int]bool)

	mappings, err := b.GetAllToolheadMappings()
	if err != nil {
		return nil, err
	}
	for printerID, printerMappings := range mappings {
		for toolheadID, mapping := range printerMappings {
			if mapping.SpoolID <= 0 {
				continue
			}
			if exclude.PrinterID != "" && printerID == exclude.PrinterID && toolheadID == exclude.ToolheadID {
				continue
			}
			assigned[mapping.SpoolID] = true
		}
	}

	spools, err := b.Spoolman.GetAllSpools()
	if err != nil {
		return nil, err
	}
	for i := range spools {
		trayID := spoolman.GetSpoolExtraString(&spools[i], spoolman.ExtraFieldActiveTray)
		if trayID == "" {
			continue
		}
		if exclude.TrayUniqueID != "" && trayID == exclude.TrayUniqueID {
			continue
		}
		assigned[spools[i].ID] = true
	}

	return assigned, nil
}

// GetAvailableSpools returns spools not assigned to any toolhead or Bambu tray.
func (b *FilamentBridge) GetAvailableSpools(exclude ExcludeAssignment) ([]spoolman.Spool, error) {
	allSpools, err := b.Spoolman.GetAllSpools()
	if err != nil {
		return nil, err
	}

	assigned, err := b.GetAssignedSpoolIDs(exclude)
	if err != nil {
		return nil, err
	}

	var available []spoolman.Spool
	for _, spool := range allSpools {
		if !assigned[spool.ID] {
			available = append(available, spool)
		}
	}
	return available, nil
}

func (b *FilamentBridge) findToolheadAssignmentForSpool(spoolID int) (printerID string, toolheadID int, found bool, err error) {
	b.Mutex.RLock()
	defer b.Mutex.RUnlock()

	return b.findToolheadAssignmentForSpoolLocked(spoolID)
}

// findToolheadAssignmentForSpoolLocked must be called with b.Mutex already held.
func (b *FilamentBridge) findToolheadAssignmentForSpoolLocked(spoolID int) (printerID string, toolheadID int, found bool, err error) {
	err = b.DB.QueryRow(
		"SELECT printer_id, toolhead_id FROM toolhead_mappings WHERE spool_id = ? LIMIT 1",
		spoolID,
	).Scan(&printerID, &toolheadID)
	if err == sql.ErrNoRows {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, err
	}
	return printerID, toolheadID, true, nil
}

func (b *FilamentBridge) findBambuTrayAssignmentForSpool(spoolID int) (trayUniqueID string, found bool, err error) {
	spool, err := b.Spoolman.GetSpool(spoolID)
	if err != nil {
		return "", false, err
	}
	trayID := spoolman.GetSpoolExtraString(spool, spoolman.ExtraFieldActiveTray)
	if trayID == "" {
		return "", false, nil
	}
	return trayID, true, nil
}

// EnsureSpoolNotAssignedElsewhere fails when the spool is already mapped to another
// Moonraker toolhead or Bambu tray (unless excluded).
func (b *FilamentBridge) EnsureSpoolNotAssignedElsewhere(spoolID int, exclude ExcludeAssignment) error {
	return b.ensureSpoolNotAssignedElsewhere(spoolID, exclude, false)
}

// ensureSpoolNotAssignedElsewhereLocked is the variant for callers that already
// hold b.Mutex (RWMutex is not reentrant).
func (b *FilamentBridge) ensureSpoolNotAssignedElsewhereLocked(spoolID int, exclude ExcludeAssignment) error {
	return b.ensureSpoolNotAssignedElsewhere(spoolID, exclude, true)
}

func (b *FilamentBridge) ensureSpoolNotAssignedElsewhere(spoolID int, exclude ExcludeAssignment, mutexHeld bool) error {
	find := b.findToolheadAssignmentForSpool
	if mutexHeld {
		find = b.findToolheadAssignmentForSpoolLocked
	}

	if printerID, toolheadID, found, err := find(spoolID); err != nil {
		return err
	} else if found {
		if exclude.PrinterID != "" && printerID == exclude.PrinterID && toolheadID == exclude.ToolheadID {
			return nil
		}
		return fmt.Errorf("spool %d is already assigned to %s toolhead %d", spoolID, b.ResolvePrinterDisplayName(printerID), toolheadID)
	}

	if trayID, found, err := b.findBambuTrayAssignmentForSpool(spoolID); err != nil {
		// Spoolman unreachable or spool unknown there — skip the Bambu tray
		// conflict check instead of blocking the local mapping.
		log.Printf("Warning: could not check Bambu tray assignment for spool %d: %v", spoolID, err)
	} else if found {
		if exclude.TrayUniqueID != "" && trayID == exclude.TrayUniqueID {
			return nil
		}
		return fmt.Errorf("spool %d is already assigned to Bambu tray %s", spoolID, trayID)
	}

	return nil
}

// RelocateSpoolFromPreviousAssignments clears Moonraker toolhead and Bambu tray
// bindings for spoolID before a new explicit assignment. The keep target is
// excluded from clearing.
func (b *FilamentBridge) RelocateSpoolFromPreviousAssignments(spoolID int, keep ExcludeAssignment) error {
	allMappings, err := b.GetAllToolheadMappings()
	if err != nil {
		return fmt.Errorf("failed to get toolhead mappings: %w", err)
	}

	for printerID, printerMappings := range allMappings {
		for toolheadID, mapping := range printerMappings {
			if mapping.SpoolID != spoolID {
				continue
			}
			if keep.PrinterID != "" && printerID == keep.PrinterID && toolheadID == keep.ToolheadID {
				continue
			}
			if err := b.UnmapToolhead(printerID, toolheadID); err != nil {
				log.Printf("Warning: Failed to unmap spool %d from %s toolhead %d during relocation: %v", spoolID, printerID, toolheadID, err)
			}
		}
	}

	if trayID, found, err := b.findBambuTrayAssignmentForSpool(spoolID); err != nil {
		log.Printf("Warning: could not check Bambu tray assignment for spool %d during relocation: %v", spoolID, err)
	} else if found {
		if keep.TrayUniqueID == "" || trayID != keep.TrayUniqueID {
			if err := b.Spoolman.UnassignSpoolFromTray(spoolID); err != nil {
				return fmt.Errorf("failed to unassign spool %d from Bambu tray: %w", spoolID, err)
			}
		}
	}

	return nil
}

// AssignSpoolToLocation assigns a spool to a location and updates Spoolman.
func (b *FilamentBridge) AssignSpoolToLocation(spoolID int, printerName string, toolheadID int, locationName string, isPrinterLocation bool) error {
	if isPrinterLocation {
		// This is a printer toolhead location: update FilaBridge toolhead mapping
		printerID, err := b.FindPrinterIDByName(printerName)
		if err != nil {
			return fmt.Errorf("failed to resolve printer %q: %w", printerName, err)
		}
		if printerID == "" {
			return fmt.Errorf("printer %q not found", printerName)
		}

		if err := b.SetToolheadMapping(printerID, toolheadID, spoolID); err != nil {
			return fmt.Errorf("failed to set toolhead mapping: %w", err)
		}

		// Get toolhead display name (custom or default)
		displayName, err := b.GetToolheadName(printerID, toolheadID)
		if err != nil || displayName == "" {
			displayName = DefaultToolheadDisplayName(toolheadID)
		}

		// Update Spoolman location; auto-created when the spool's location field is updated.
		spoolmanLocation := fmt.Sprintf("%s - %s", printerName, displayName)

		if err := b.Spoolman.UpdateSpoolLocation(spoolID, spoolmanLocation); err != nil {
			// FilaBridge mapping is more critical — log but don't fail the operation
			log.Printf("Warning: Failed to update Spoolman location for spool %d: %v", spoolID, err)
		}

		log.Printf("Successfully assigned spool %d to %s toolhead %d (%s)", spoolID, printerName, toolheadID, displayName)
	} else {
		// This is a non-printer location (drybox, storage, etc.)
		if err := b.RelocateSpoolFromPreviousAssignments(spoolID, ExcludeAssignment{}); err != nil {
			return err
		}

		if locationName == "" {
			return fmt.Errorf("location name cannot be empty")
		}

		if _, err := b.Spoolman.GetOrCreateLocation(locationName); err != nil {
			log.Printf("Warning: Failed to create/verify location '%s' in Spoolman: %v", locationName, err)
		}

		if err := b.Spoolman.UpdateSpoolLocation(spoolID, locationName); err != nil {
			return fmt.Errorf("failed to update Spoolman location for spool %d: %w", spoolID, err)
		}

		log.Printf("Successfully assigned spool %d to location '%s'", spoolID, locationName)
	}

	return nil
}

// clearSpoolFromAllToolheads removes a spool from all toolhead mappings.
func (b *FilamentBridge) clearSpoolFromAllToolheads(spoolID int) error {
	allMappings, err := b.GetAllToolheadMappings()
	if err != nil {
		return fmt.Errorf("failed to get toolhead mappings: %w", err)
	}

	for printerID, printerMappings := range allMappings {
		for toolheadID, mapping := range printerMappings {
			if mapping.SpoolID == spoolID {
				if err := b.UnmapToolhead(printerID, toolheadID); err != nil {
					log.Printf("Warning: Failed to unmap spool %d from %s toolhead %d: %v", spoolID, printerID, toolheadID, err)
				} else {
					log.Printf("Cleared spool %d from %s toolhead %d", spoolID, printerID, toolheadID)
				}
			}
		}
	}

	return nil
}
