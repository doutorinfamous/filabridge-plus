package core

import (
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

// slotID resolves the exclusion to a printer_slots key (empty when unset).
func (e ExcludeAssignment) slotID() string {
	if e.TrayUniqueID != "" {
		return e.TrayUniqueID
	}
	if e.PrinterID != "" {
		return ToolheadSlotID(e.PrinterID, e.ToolheadID)
	}
	return ""
}

// GetAssignedSpoolIDs returns spool IDs in use across all printer slots
// (Moonraker toolheads and Bambu AMS trays).
func (b *FilamentBridge) GetAssignedSpoolIDs(exclude ExcludeAssignment) (map[int]bool, error) {
	excludeSlotID := exclude.slotID()

	rows, err := b.DB.Query(
		"SELECT slot_id, spool_id FROM printer_slots WHERE spool_id IS NOT NULL AND spool_id > 0",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get assigned spools: %w", err)
	}
	defer rows.Close()

	assigned := make(map[int]bool)
	for rows.Next() {
		var slotID string
		var spoolID int
		if err := rows.Scan(&slotID, &spoolID); err != nil {
			return nil, fmt.Errorf("failed to scan assigned spool row: %w", err)
		}
		if excludeSlotID != "" && slotID == excludeSlotID {
			continue
		}
		assigned[spoolID] = true
	}

	return assigned, rows.Err()
}

// GetAvailableSpools returns spools not assigned to any printer slot.
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

// EnsureSpoolNotAssignedElsewhere fails when the spool is already mapped to
// another printer slot (unless excluded).
func (b *FilamentBridge) EnsureSpoolNotAssignedElsewhere(spoolID int, exclude ExcludeAssignment) error {
	slots, err := b.FindSlotsBySpool(spoolID)
	if err != nil {
		return err
	}

	excludeSlotID := exclude.slotID()
	for _, slot := range slots {
		if excludeSlotID != "" && slot.SlotID == excludeSlotID {
			continue
		}
		if slot.SlotType == SlotTypeToolhead && slot.ToolheadID != nil {
			return fmt.Errorf("spool %d is already assigned to %s toolhead %d", spoolID, b.ResolvePrinterDisplayName(slot.PrinterID), *slot.ToolheadID)
		}
		return fmt.Errorf("spool %d is already assigned to Bambu tray %s", spoolID, slot.SlotID)
	}

	return nil
}

// RelocateSpoolFromPreviousAssignments clears all slot bindings for spoolID
// before a new explicit assignment. The keep target is excluded from clearing.
// SQLite is the source of truth; the Spoolman extra.active_tray mirror is
// cleared best-effort.
func (b *FilamentBridge) RelocateSpoolFromPreviousAssignments(spoolID int, keep ExcludeAssignment) error {
	cleared, err := b.ClearSpoolFromSlots(spoolID, keep.slotID())
	if err != nil {
		return fmt.Errorf("failed to clear previous slot assignments: %w", err)
	}
	for _, slot := range cleared {
		log.Printf("Cleared spool %d from %s slot %s during relocation", spoolID, slot.PrinterID, slot.SlotID)
	}

	// Mirror: clear stale active_tray in Spoolman (covers slots cleared above
	// and legacy assignments that only exist on the Spoolman side).
	if trayID, found, err := b.findSpoolmanActiveTray(spoolID); err != nil {
		log.Printf("Warning: could not check Spoolman active_tray for spool %d during relocation: %v", spoolID, err)
	} else if found {
		if keep.TrayUniqueID == "" || trayID != keep.TrayUniqueID {
			if err := b.Spoolman.UnassignSpoolFromTray(spoolID); err != nil {
				log.Printf("Warning: failed to clear Spoolman active_tray for spool %d: %v", spoolID, err)
			}
		}
	}

	return nil
}

// findSpoolmanActiveTray returns the tray id stored in Spoolman extra.active_tray.
func (b *FilamentBridge) findSpoolmanActiveTray(spoolID int) (trayUniqueID string, found bool, err error) {
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
