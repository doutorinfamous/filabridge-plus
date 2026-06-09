package main

import (
	"fmt"
	"log"
)

func findSpoolIDForBambuTray(spools []SpoolmanSpool, tray BambuTray) int {
	for i := range spools {
		activeTray := GetSpoolExtraString(&spools[i], spoolExtraFieldActiveTray)
		if activeTray == "" {
			continue
		}
		if activeTray == tray.UniqueID || activeTray == tray.EntityID {
			return spools[i].ID
		}
	}
	return 0
}

// AssignSpoolToBambuTray assigns a spool to a Bambu AMS tray via Spoolman extra.active_tray.
func (b *FilamentBridge) AssignSpoolToBambuTray(spoolID int, trayUniqueID, displayName string) error {
	if err := b.ensureSpoolNotAssignedElsewhere(spoolID, ExcludeAssignment{TrayUniqueID: trayUniqueID}); err != nil {
		return err
	}

	if err := b.spoolman.EnsureSpoolExtraFields(); err != nil {
		log.Printf("Warning: failed to ensure spool extra fields: %v", err)
	}
	if err := b.spoolman.AssignSpoolToTray(spoolID, trayUniqueID); err != nil {
		return fmt.Errorf("failed to assign spool to tray: %w", err)
	}
	if displayName != "" {
		if err := b.spoolman.UpdateSpoolLocation(spoolID, displayName); err != nil {
			log.Printf("Warning: failed to update spool location for #%d: %v", spoolID, err)
		}
	}
	return nil
}

// UnassignBambuTray clears the spool assigned to a tray and moves it to default storage when configured.
func (b *FilamentBridge) UnassignBambuTray(trayUniqueID string) error {
	spool, err := b.spoolman.FindSpoolByActiveTray("", trayUniqueID)
	if err != nil {
		return err
	}
	if spool == nil {
		return nil
	}
	spoolID := spool.ID
	if err := b.spoolman.UnassignSpoolFromTray(spoolID); err != nil {
		return err
	}
	b.tryAutoAssignSpoolToDefaultStorage(spoolID)
	return nil
}

// EnrichBambuPrintersWithAssignments adds Spoolman assignment info to discovered printers.
func (b *FilamentBridge) EnrichBambuPrintersWithAssignments(printers []BambuPrinter) ([]BambuPrinter, error) {
	spools, err := b.spoolman.GetAllSpools()
	if err != nil {
		return printers, err
	}

	registered, _ := b.GetBambuPrinterConfigs()

	for i := range printers {
		for id, cfg := range registered {
			if cfg.HADeviceID == printers[i].DeviceID || normalizeBambuHAPrefix(cfg.HAPrefix) == normalizeBambuHAPrefix(printers[i].Prefix) {
				printers[i].Registered = true
				printers[i].PrinterID = id
			}
		}
		for j := range printers[i].ExternalSpools {
			if sid := findSpoolIDForBambuTray(spools, printers[i].ExternalSpools[j]); sid > 0 {
				printers[i].ExternalSpools[j].AssignedSpoolID = &sid
			}
		}
		for j := range printers[i].AMSUnits {
			for k := range printers[i].AMSUnits[j].Trays {
				if sid := findSpoolIDForBambuTray(spools, printers[i].AMSUnits[j].Trays[k]); sid > 0 {
					printers[i].AMSUnits[j].Trays[k].AssignedSpoolID = &sid
				}
			}
		}
	}
	return printers, nil
}
