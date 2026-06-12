package bambu

import (
	"fmt"
	"log"

	"filabridge/core"
)

// AssignSpoolToTray assigns a spool to a Bambu AMS tray. The printer_slots
// table is the source of truth; Spoolman (extra.active_tray + location) is
// updated as a best-effort mirror.
func AssignSpoolToTray(b *core.FilamentBridge, spoolID int, trayUniqueID, displayName string) error {
	if err := b.RelocateSpoolFromPreviousAssignments(spoolID, core.ExcludeAssignment{TrayUniqueID: trayUniqueID}); err != nil {
		return err
	}

	printerID, err := FindTrayPrinterID(b, trayUniqueID)
	if err != nil {
		log.Printf("Warning: failed to resolve printer for tray %s: %v", trayUniqueID, err)
	}
	if err := b.SetSlotSpool(trayUniqueID, printerID, core.SlotTypeAMSTray, displayName, spoolID); err != nil {
		return fmt.Errorf("failed to assign spool to tray: %w", err)
	}

	// Mirror to Spoolman (best-effort).
	if err := b.Spoolman.EnsureSpoolExtraFields(); err != nil {
		log.Printf("Warning: failed to ensure spool extra fields: %v", err)
	}
	if err := b.Spoolman.AssignSpoolToTray(spoolID, trayUniqueID); err != nil {
		log.Printf("Warning: failed to mirror tray assignment to Spoolman for spool #%d: %v", spoolID, err)
	}
	if displayName != "" {
		if err := b.Spoolman.UpdateSpoolLocation(spoolID, displayName); err != nil {
			log.Printf("Warning: failed to update spool location for #%d: %v", spoolID, err)
		}
	}
	return nil
}

// FindSpoolIDForTrayLocal returns the spool assigned to a tray in printer_slots
// (0 when the tray is empty or unknown).
func FindSpoolIDForTrayLocal(b *core.FilamentBridge, trayUniqueID string) (int, error) {
	slot, err := b.GetSlot(trayUniqueID)
	if err != nil {
		return 0, err
	}
	if slot == nil {
		return 0, nil
	}
	return slot.SpoolID, nil
}

// UnassignTray clears the spool assigned to a tray and moves it to default storage when configured.
func UnassignTray(b *core.FilamentBridge, trayUniqueID string) error {
	spoolID, err := FindSpoolIDForTrayLocal(b, trayUniqueID)
	if err != nil {
		return err
	}
	if spoolID == 0 {
		// Legacy fallback: assignment may only exist on the Spoolman side.
		spool, err := b.Spoolman.FindSpoolByActiveTray("", trayUniqueID)
		if err != nil || spool == nil {
			return err
		}
		spoolID = spool.ID
	}

	if err := b.ClearSlotSpool(trayUniqueID); err != nil {
		return err
	}
	if err := b.Spoolman.UnassignSpoolFromTray(spoolID); err != nil {
		log.Printf("Warning: failed to mirror tray unassignment to Spoolman for spool #%d: %v", spoolID, err)
	}
	b.TryAutoAssignSpoolToDefaultStorage(spoolID)
	return nil
}

// EnrichPrintersWithAssignments adds local slot assignment info to discovered printers.
func EnrichPrintersWithAssignments(b *core.FilamentBridge, printers []Printer) ([]Printer, error) {
	registered, _ := b.GetBambuPrinterConfigs()

	enrich := func(tray *Tray) {
		spoolID, err := FindSpoolIDForTrayLocal(b, tray.UniqueID)
		if err != nil {
			log.Printf("Warning: failed to look up spool for tray %s: %v", tray.UniqueID, err)
			return
		}
		if spoolID > 0 {
			tray.AssignedSpoolID = &spoolID
		}
	}

	for i := range printers {
		for id, cfg := range registered {
			if cfg.HADeviceID == printers[i].DeviceID || NormalizePrefix(cfg.HAPrefix) == NormalizePrefix(printers[i].Prefix) {
				printers[i].Registered = true
				printers[i].PrinterID = id
			}
		}
		for j := range printers[i].ExternalSpools {
			enrich(&printers[i].ExternalSpools[j])
		}
		for j := range printers[i].AMSUnits {
			for k := range printers[i].AMSUnits[j].Trays {
				enrich(&printers[i].AMSUnits[j].Trays[k])
			}
		}
	}
	return printers, nil
}
