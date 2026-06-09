package main

import (
	"database/sql"
	"fmt"
)

// ExcludeAssignment identifies a mapping context whose current spool stays selectable.
type ExcludeAssignment struct {
	PrinterName  string
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
	for printerName, printerMappings := range mappings {
		for toolheadID, mapping := range printerMappings {
			if mapping.SpoolID <= 0 {
				continue
			}
			if exclude.PrinterName != "" && printerName == exclude.PrinterName && toolheadID == exclude.ToolheadID {
				continue
			}
			assigned[mapping.SpoolID] = true
		}
	}

	spools, err := b.spoolman.GetAllSpools()
	if err != nil {
		return nil, err
	}
	for i := range spools {
		trayID := GetSpoolExtraString(&spools[i], spoolExtraFieldActiveTray)
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
func (b *FilamentBridge) GetAvailableSpools(exclude ExcludeAssignment) ([]SpoolmanSpool, error) {
	allSpools, err := b.spoolman.GetAllSpools()
	if err != nil {
		return nil, err
	}

	assigned, err := b.GetAssignedSpoolIDs(exclude)
	if err != nil {
		return nil, err
	}

	var available []SpoolmanSpool
	for _, spool := range allSpools {
		if !assigned[spool.ID] {
			available = append(available, spool)
		}
	}
	return available, nil
}

func (b *FilamentBridge) findToolheadAssignmentForSpool(spoolID int) (printerName string, toolheadID int, found bool, err error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	err = b.db.QueryRow(
		"SELECT printer_name, toolhead_id FROM toolhead_mappings WHERE spool_id = ? LIMIT 1",
		spoolID,
	).Scan(&printerName, &toolheadID)
	if err == sql.ErrNoRows {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, err
	}
	return printerName, toolheadID, true, nil
}

func (b *FilamentBridge) findBambuTrayAssignmentForSpool(spoolID int) (trayUniqueID string, found bool, err error) {
	spool, err := b.spoolman.GetSpool(spoolID)
	if err != nil {
		return "", false, err
	}
	trayID := GetSpoolExtraString(spool, spoolExtraFieldActiveTray)
	if trayID == "" {
		return "", false, nil
	}
	return trayID, true, nil
}

func (b *FilamentBridge) ensureSpoolNotAssignedElsewhere(spoolID int, exclude ExcludeAssignment) error {
	if printerName, toolheadID, found, err := b.findToolheadAssignmentForSpool(spoolID); err != nil {
		return err
	} else if found {
		if exclude.PrinterName != "" && printerName == exclude.PrinterName && toolheadID == exclude.ToolheadID {
			return nil
		}
		return fmt.Errorf("spool %d is already assigned to %s toolhead %d", spoolID, printerName, toolheadID)
	}

	if trayID, found, err := b.findBambuTrayAssignmentForSpool(spoolID); err != nil {
		return err
	} else if found {
		if exclude.TrayUniqueID != "" && trayID == exclude.TrayUniqueID {
			return nil
		}
		return fmt.Errorf("spool %d is already assigned to Bambu tray %s", spoolID, trayID)
	}

	return nil
}
