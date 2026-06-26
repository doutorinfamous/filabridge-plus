package server

import (
	"log"

	"filabridge/snapmaker"
)

func (ws *WebServer) syncSnapmakerFilamentAfterToolheadMap(printerName string, toolheadID, spoolID int) {
	printerID, err := ws.bridge.FindPrinterIDByName(printerName)
	if err != nil {
		log.Printf("Warning: failed to resolve printer %q for Snapmaker filament sync: %v", printerName, err)
		return
	}
	if printerID == "" {
		return
	}

	if err := snapmaker.TrySyncFilamentToToolhead(ws.bridge, printerID, toolheadID, spoolID); err != nil {
		log.Printf("Warning: Snapmaker filament sync failed for %s toolhead %d: %v", printerName, toolheadID, err)
	}
}
