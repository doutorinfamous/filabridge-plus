package bambu

import (
	"fmt"
	"log"

	"filabridge/core"
	"filabridge/spoolman"
)

func findSpoolIDForTray(spools []spoolman.Spool, tray Tray) int {
	for i := range spools {
		activeTray := spoolman.GetSpoolExtraString(&spools[i], spoolman.ExtraFieldActiveTray)
		if activeTray == "" {
			continue
		}
		if activeTray == tray.UniqueID || activeTray == tray.EntityID {
			return spools[i].ID
		}
	}
	return 0
}

// AssignSpoolToTray assigns a spool to a Bambu AMS tray via Spoolman extra.active_tray.
func AssignSpoolToTray(b *core.FilamentBridge, spoolID int, trayUniqueID, displayName string) error {
	if err := b.RelocateSpoolFromPreviousAssignments(spoolID, core.ExcludeAssignment{TrayUniqueID: trayUniqueID}); err != nil {
		return err
	}

	if err := b.Spoolman.EnsureSpoolExtraFields(); err != nil {
		log.Printf("Warning: failed to ensure spool extra fields: %v", err)
	}
	if err := b.Spoolman.AssignSpoolToTray(spoolID, trayUniqueID); err != nil {
		return fmt.Errorf("failed to assign spool to tray: %w", err)
	}
	if displayName != "" {
		if err := b.Spoolman.UpdateSpoolLocation(spoolID, displayName); err != nil {
			log.Printf("Warning: failed to update spool location for #%d: %v", spoolID, err)
		}
	}
	return nil
}

// UnassignTray clears the spool assigned to a tray and moves it to default storage when configured.
func UnassignTray(b *core.FilamentBridge, trayUniqueID string) error {
	spool, err := b.Spoolman.FindSpoolByActiveTray("", trayUniqueID)
	if err != nil {
		return err
	}
	if spool == nil {
		return nil
	}
	spoolID := spool.ID
	if err := b.Spoolman.UnassignSpoolFromTray(spoolID); err != nil {
		return err
	}
	b.TryAutoAssignSpoolToDefaultStorage(spoolID)
	return nil
}

// EnrichPrintersWithAssignments adds Spoolman assignment info to discovered printers.
func EnrichPrintersWithAssignments(b *core.FilamentBridge, printers []Printer) ([]Printer, error) {
	spools, err := b.Spoolman.GetAllSpools()
	if err != nil {
		return printers, err
	}

	registered, _ := b.GetBambuPrinterConfigs()

	for i := range printers {
		for id, cfg := range registered {
			if cfg.HADeviceID == printers[i].DeviceID || NormalizePrefix(cfg.HAPrefix) == NormalizePrefix(printers[i].Prefix) {
				printers[i].Registered = true
				printers[i].PrinterID = id
			}
		}
		for j := range printers[i].ExternalSpools {
			if sid := findSpoolIDForTray(spools, printers[i].ExternalSpools[j]); sid > 0 {
				printers[i].ExternalSpools[j].AssignedSpoolID = &sid
			}
		}
		for j := range printers[i].AMSUnits {
			for k := range printers[i].AMSUnits[j].Trays {
				if sid := findSpoolIDForTray(spools, printers[i].AMSUnits[j].Trays[k]); sid > 0 {
					printers[i].AMSUnits[j].Trays[k].AssignedSpoolID = &sid
				}
			}
		}
	}
	return printers, nil
}
