package bambu

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/skip2/go-qrcode"

	"filabridge/core"
)

var (
	amsLocationRegex      = regexp.MustCompile(`^(.+?)\s+-\s+AMS(?:\s+HT(?:\s+(\d+))?|\s+(\d+))?\s+Slot\s+(\d+)$`)
	externalLocationRegex = regexp.MustCompile(`^(.+?)\s+-\s+External Spool$`)
)

// IsBambuLocation returns true if the location string matches a Bambu AMS/external format.
func IsBambuLocation(location string) bool {
	return amsLocationRegex.MatchString(location) || externalLocationRegex.MatchString(location)
}

// ParseLocation resolves a display location to a cached tray.
func ParseLocation(b *core.FilamentBridge, location string) (*Tray, error) {
	location = strings.TrimSpace(location)
	tray, err := FindTrayByDisplayName(b, location)
	if err != nil {
		return nil, err
	}
	if tray != nil {
		return tray, nil
	}

	if m := externalLocationRegex.FindStringSubmatch(location); len(m) == 2 {
		printerName := strings.TrimSpace(m[1])
		return findTrayByPrinterAndType(b, printerName, true, 0, 0)
	}
	if m := amsLocationRegex.FindStringSubmatch(location); len(m) == 5 {
		printerName := strings.TrimSpace(m[1])
		slotNum := 0
		fmt.Sscanf(m[4], "%d", &slotNum)
		amsNum := 1
		if m[3] != "" {
			fmt.Sscanf(m[3], "%d", &amsNum)
		} else if m[2] != "" {
			htNum := 0
			fmt.Sscanf(m[2], "%d", &htNum)
			amsNum = 127 + htNum
		}
		return findTrayByPrinterAndType(b, printerName, false, amsNum, slotNum)
	}
	return nil, nil
}

func findTrayByPrinterAndType(b *core.FilamentBridge, printerName string, isExternal bool, amsNumber, trayNumber int) (*Tray, error) {
	configs, err := b.GetBambuPrinterConfigs()
	if err != nil {
		return nil, err
	}
	for id, cfg := range configs {
		if cfg.Name != printerName {
			continue
		}
		trays, err := GetTraysForPrinter(b, id)
		if err != nil {
			return nil, err
		}
		for i := range trays {
			if isExternal && trays[i].IsExternal {
				return &trays[i], nil
			}
			if !isExternal && !trays[i].IsExternal && trays[i].AMSNumber == amsNumber && trays[i].TrayNumber == trayNumber {
				return &trays[i], nil
			}
		}
	}
	return nil, nil
}

// GenerateNFCURLs builds NFC/QR URL entries for all registered Bambu trays.
// baseURL must be a full base URL like "http://192.168.1.20:5000" (no trailing slash).
func GenerateNFCURLs(b *core.FilamentBridge, baseURL string) ([]map[string]interface{}, error) {
	configs, err := b.GetBambuPrinterConfigs()
	if err != nil {
		return nil, err
	}

	var urls []map[string]interface{}
	for printerID, cfg := range configs {
		trays, err := GetTraysForPrinter(b, printerID)
		if err != nil {
			continue
		}
		for _, tray := range trays {
			locationParam := tray.DisplayName
			if locationParam == "" {
				locationParam = FormatTrayDisplayName(cfg.Name, tray.AMSNumber, tray.TrayNumber, tray.IsExternal)
			}
			nfcURL := fmt.Sprintf("%s/api/nfc/assign?location=%s", baseURL, url.QueryEscape(locationParam))
			entry := map[string]interface{}{
				"type":           "location",
				"location_type":  "ams_slot",
				"location_name":  locationParam,
				"display_name":   locationParam,
				"printer_name":   cfg.Name,
				"printer_id":     printerID,
				"tray_unique_id": tray.UniqueID,
				"url":            nfcURL,
				"qr_code_base64": "",
			}
			if qr, err := qrcode.Encode(nfcURL, qrcode.Medium, 256); err == nil {
				entry["qr_code_base64"] = base64.StdEncoding.EncodeToString(qr)
			}
			urls = append(urls, entry)
		}
	}
	return urls, nil
}
