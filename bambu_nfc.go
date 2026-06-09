package main

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/skip2/go-qrcode"
)

var (
	bambuAMSLocationRegex     = regexp.MustCompile(`^(.+?)\s+-\s+AMS(?:\s+HT(?:\s+(\d+))?|\s+(\d+))?\s+Slot\s+(\d+)$`)
	bambuExternalLocationRegex = regexp.MustCompile(`^(.+?)\s+-\s+External Spool$`)
)

// IsBambuLocation returns true if the location string matches a Bambu AMS/external format.
func IsBambuLocation(location string) bool {
	return bambuAMSLocationRegex.MatchString(location) || bambuExternalLocationRegex.MatchString(location)
}

// ParseBambuLocation resolves a display location to a cached tray.
func (b *FilamentBridge) ParseBambuLocation(location string) (*BambuTray, error) {
	location = strings.TrimSpace(location)
	tray, err := b.FindBambuTrayByDisplayName(location)
	if err != nil {
		return nil, err
	}
	if tray != nil {
		return tray, nil
	}

	if m := bambuExternalLocationRegex.FindStringSubmatch(location); len(m) == 2 {
		printerName := strings.TrimSpace(m[1])
		return b.findBambuTrayByPrinterAndType(printerName, true, 0, 0)
	}
	if m := bambuAMSLocationRegex.FindStringSubmatch(location); len(m) == 5 {
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
		return b.findBambuTrayByPrinterAndType(printerName, false, amsNum, slotNum)
	}
	return nil, nil
}

func (b *FilamentBridge) findBambuTrayByPrinterAndType(printerName string, isExternal bool, amsNumber, trayNumber int) (*BambuTray, error) {
	configs, err := b.GetBambuPrinterConfigs()
	if err != nil {
		return nil, err
	}
	for id, cfg := range configs {
		if cfg.Name != printerName {
			continue
		}
		trays, err := b.GetBambuTraysForPrinter(id)
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

// GenerateBambuNFCURLs builds NFC/QR URL entries for all registered Bambu trays.
func (b *FilamentBridge) GenerateBambuNFCURLs(host string) ([]map[string]interface{}, error) {
	configs, err := b.GetBambuPrinterConfigs()
	if err != nil {
		return nil, err
	}

	var urls []map[string]interface{}
	for printerID, cfg := range configs {
		trays, err := b.GetBambuTraysForPrinter(printerID)
		if err != nil {
			continue
		}
		for _, tray := range trays {
			locationParam := tray.DisplayName
			if locationParam == "" {
				locationParam = formatBambuTrayDisplayName(cfg.Name, tray.AMSNumber, tray.TrayNumber, tray.IsExternal)
			}
			nfcURL := fmt.Sprintf("http://%s/api/nfc/assign?location=%s", host, url.QueryEscape(locationParam))
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
