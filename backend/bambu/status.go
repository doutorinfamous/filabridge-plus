package bambu

import (
	"log"

	"filabridge/core"
)

// BuildPrinterData fetches live HA status for a registered Bambu printer.
func BuildPrinterData(b *core.FilamentBridge, printerID string, config core.PrinterConfig) core.PrinterData {
	data := core.PrinterData{
		Name:  config.Name,
		State: core.StateOffline,
	}

	ha, err := NewHAClientFromConfig(b)
	if err != nil {
		log.Printf("Warning: Bambu printer %s (%s): %v", printerID, config.Name, err)
		return data
	}

	printers, err := DiscoverPrinters(ha)
	if err != nil {
		log.Printf("Warning: Failed to discover Bambu printers for %s: %v", config.Name, err)
		return data
	}

	for i := range printers {
		if printers[i].DeviceID == config.HADeviceID ||
			NormalizePrefix(printers[i].Prefix) == NormalizePrefix(config.HAPrefix) {
			return PrinterToPrinterData(printers[i])
		}
	}

	log.Printf("Warning: Bambu printer %s not found in Home Assistant discovery", config.Name)
	return data
}
