package main

import "log"

// buildBambuPrinterData fetches live HA status for a registered Bambu printer.
func (b *FilamentBridge) buildBambuPrinterData(printerID string, config PrinterConfig) PrinterData {
	data := PrinterData{
		Name:  config.Name,
		State: StateOffline,
	}

	ha, err := b.NewHAClientFromConfig()
	if err != nil {
		log.Printf("Warning: Bambu printer %s (%s): %v", printerID, config.Name, err)
		return data
	}

	printers, err := DiscoverBambuPrinters(ha)
	if err != nil {
		log.Printf("Warning: Failed to discover Bambu printers for %s: %v", config.Name, err)
		return data
	}

	for i := range printers {
		if printers[i].DeviceID == config.HADeviceID ||
			normalizeBambuHAPrefix(printers[i].Prefix) == normalizeBambuHAPrefix(config.HAPrefix) {
			return bambuPrinterToPrinterData(printers[i])
		}
	}

	log.Printf("Warning: Bambu printer %s not found in Home Assistant discovery", config.Name)
	return data
}
