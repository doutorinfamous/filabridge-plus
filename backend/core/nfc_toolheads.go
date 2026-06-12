package core

import (
	"encoding/base64"
	"fmt"
	"net/url"

	"github.com/skip2/go-qrcode"
)

// GenerateToolheadNFCURLs builds NFC/QR URL entries for all Moonraker toolheads,
// including empty slots (mirrors bambu.GenerateNFCURLs for AMS trays).
func GenerateToolheadNFCURLs(b *FilamentBridge, baseURL string) ([]map[string]interface{}, error) {
	configs, err := b.GetAllPrinterConfigs()
	if err != nil {
		return nil, err
	}

	var urls []map[string]interface{}
	for printerID, cfg := range configs {
		if cfg.Driver != "" && cfg.Driver != DriverMoonraker {
			continue
		}
		if cfg.Toolheads < 1 {
			continue
		}

		for toolheadID := 0; toolheadID < cfg.Toolheads; toolheadID++ {
			displayName, err := b.GetToolheadName(printerID, toolheadID)
			if err != nil || displayName == "" {
				displayName = DefaultToolheadDisplayName(toolheadID)
			}

			locationParam := fmt.Sprintf("%s - %s", cfg.Name, displayName)
			nfcURL := fmt.Sprintf("%s/api/nfc/assign?location=%s", baseURL, url.QueryEscape(locationParam))
			entry := map[string]interface{}{
				"type":                  "location",
				"location_type":         "toolhead",
				"location_name":         locationParam,
				"display_name":          locationParam,
				"printer_name":          cfg.Name,
				"printer_id":            printerID,
				"toolhead_display_name": displayName,
				"url":                   nfcURL,
				"qr_code_base64":        "",
			}
			if qr, err := qrcode.Encode(nfcURL, qrcode.Medium, 256); err == nil {
				entry["qr_code_base64"] = base64.StdEncoding.EncodeToString(qr)
			}
			urls = append(urls, entry)
		}
	}

	return urls, nil
}
