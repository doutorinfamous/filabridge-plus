package server

import (
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	neturl "net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/skip2/go-qrcode"

	"filabridge/bambu"
	"filabridge/core"
	"filabridge/nfc"
	"filabridge/spoolman"
)

const nfcScanPath = "/nfc/scan"

func nfcSessionIDFromRequest(c *gin.Context) string {
	if sid := strings.TrimSpace(c.Query("session_id")); sid != "" {
		return sid
	}
	return nfc.SessionIDForIP(nfc.ClientIP(c.ClientIP()))
}

func redirectNFCScan(c *gin.Context, query neturl.Values) {
	target := nfcScanPath
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	c.Header("Cache-Control", "no-store")
	c.Redirect(http.StatusFound, target)
}

func redirectNFCScanPending(c *gin.Context, session *nfc.Session) {
	q := neturl.Values{}
	if session != nil && session.SessionID != "" {
		q.Set("session_id", session.SessionID)
	}
	redirectNFCScan(c, q)
}

func redirectNFCScanError(c *gin.Context, message string) {
	q := neturl.Values{}
	q.Set("error", message)
	redirectNFCScan(c, q)
}

func redirectNFCScanSuccess(c *gin.Context, ws *WebServer, session *nfc.Session, toolheadDisplayName string) {
	q := neturl.Values{}
	q.Set("success", "1")
	q.Set("spool_id", strconv.Itoa(session.SpoolID))

	spoolMeta := buildNFCSessionSpoolMeta(ws, session.SpoolID)
	if name, ok := spoolMeta["name"].(string); ok && name != "" {
		q.Set("spool_name", name)
	}
	if material, ok := spoolMeta["material"].(string); ok && material != "" {
		q.Set("spool_material", material)
	}
	if brand, ok := spoolMeta["brand"].(string); ok && brand != "" {
		q.Set("spool_brand", brand)
	}
	if color, ok := spoolMeta["color_hex"].(string); ok && color != "" {
		q.Set("spool_color", color)
	}
	if weight, ok := spoolMeta["remaining_weight"].(float64); ok && weight > 0 {
		q.Set("spool_weight", strconv.FormatFloat(weight, 'f', 0, 64))
	}

	if bambu.IsBambuLocation(session.LocationName) {
		q.Set("location_type", "ams_slot")
		q.Set("location", session.LocationName)
		if idx := strings.Index(session.LocationName, " - "); idx >= 0 {
			q.Set("printer", session.LocationName[:idx])
			q.Set("slot", strings.TrimSpace(session.LocationName[idx+3:]))
		}
	} else if session.IsPrinterLocation && session.PrinterName != "" {
		q.Set("location_type", "toolhead")
		q.Set("location", session.LocationName)
		q.Set("printer", session.PrinterName)
		q.Set("toolhead", toolheadDisplayName)
	} else {
		q.Set("location_type", "storage")
		q.Set("location", session.LocationName)
	}

	redirectNFCScan(c, q)
}

func spoolColorHex(spool *spoolman.Spool) string {
	if spool == nil || spool.Filament == nil || spool.Filament.ColorHex == "" {
		return ""
	}
	colorHex := spool.Filament.ColorHex
	if !strings.HasPrefix(colorHex, "#") {
		colorHex = "#" + colorHex
	}
	return colorHex
}

func buildNFCSessionSpoolMeta(ws *WebServer, spoolID int) gin.H {
	spool, err := ws.bridge.Spoolman.GetSpool(spoolID)
	if err != nil {
		log.Printf("Warning: failed to load spool %d for NFC session: %v", spoolID, err)
		return gin.H{
			"id": spoolID,
		}
	}

	meta := gin.H{
		"id":       spool.ID,
		"name":     spool.Name,
		"material": spool.Material,
		"brand":    spool.Brand,
	}
	if color := spoolColorHex(spool); color != "" {
		meta["color_hex"] = color
	}
	if spool.RemainingWeight > 0 {
		meta["remaining_weight"] = spool.RemainingWeight
	}
	return meta
}

func resolveToolheadDisplayName(ws *WebServer, session *nfc.Session) string {
	toolheadDisplayName := core.DefaultToolheadDisplayName(session.ToolheadID)
	printerConfigs, err := ws.bridge.GetAllPrinterConfigs()
	if err != nil {
		return toolheadDisplayName
	}
	for printerID, cfg := range printerConfigs {
		if cfg.Name == session.PrinterName {
			if name, nameErr := ws.bridge.GetToolheadName(printerID, session.ToolheadID); nameErr == nil {
				return name
			}
			break
		}
	}
	return toolheadDisplayName
}

func filamentColorHex(filament *spoolman.Filament) string {
	if filament == nil || filament.ColorHex == "" {
		return ""
	}
	colorHex := filament.ColorHex
	if !strings.HasPrefix(colorHex, "#") {
		colorHex = "#" + colorHex
	}
	return colorHex
}

func buildNFCSessionFilamentMeta(filament *spoolman.Filament) gin.H {
	if filament == nil {
		return nil
	}

	meta := gin.H{
		"id":       filament.ID,
		"name":     filament.Name,
		"material": filament.Material,
	}
	if filament.Vendor != nil && filament.Vendor.Name != "" {
		meta["brand"] = filament.Vendor.Name
	}
	if color := filamentColorHex(filament); color != "" {
		meta["color_hex"] = color
	}
	return meta
}

func buildNFCSessionSpoolCandidateMeta(spool spoolman.Spool) gin.H {
	meta := gin.H{
		"id":               spool.ID,
		"name":             spool.Name,
		"material":         spool.Material,
		"brand":            spool.Brand,
		"remaining_weight": spool.RemainingWeight,
	}
	if color := spoolColorHex(&spool); color != "" {
		meta["color_hex"] = color
	}
	if spool.Location != "" {
		meta["location"] = spool.Location
	}
	return meta
}

func buildNFCSessionPendingFilamentMeta(ws *WebServer, session *nfc.Session) gin.H {
	if !session.HasPendingFilament {
		return nil
	}

	filament, err := ws.bridge.Spoolman.GetFilament(session.PendingFilamentID)
	if err != nil {
		log.Printf("Warning: failed to load filament %d for NFC session: %v", session.PendingFilamentID, err)
		return gin.H{
			"id": session.PendingFilamentID,
		}
	}

	spools, err := ws.bridge.Spoolman.GetSpoolsByFilament(session.PendingFilamentID)
	if err != nil {
		log.Printf("Warning: failed to load spools for filament %d: %v", session.PendingFilamentID, err)
		spools = []spoolman.Spool{}
	}

	candidates := make([]gin.H, 0, len(spools))
	for _, spool := range spools {
		candidates = append(candidates, buildNFCSessionSpoolCandidateMeta(spool))
	}

	meta := buildNFCSessionFilamentMeta(filament)
	meta["candidates"] = candidates
	return meta
}

func (ws *WebServer) completeNFCAssignment(session *nfc.Session) error {
	if bambu.IsBambuLocation(session.LocationName) {
		tray, trayErr := bambu.ParseLocation(ws.bridge, session.LocationName)
		if trayErr != nil || tray == nil {
			return fmt.Errorf("failed to resolve Bambu tray location")
		}
		return bambu.AssignSpoolToTray(ws.bridge, session.SpoolID, tray.UniqueID, session.LocationName)
	}
	return ws.bridge.AssignSpoolToLocation(session.SpoolID, session.PrinterName, session.ToolheadID, session.LocationName, session.IsPrinterLocation)
}

func buildNFCSelectSpoolSuccessPayload(ws *WebServer, session *nfc.Session) gin.H {
	toolheadDisplayName := resolveToolheadDisplayName(ws, session)
	success := gin.H{
		"spool_id": session.SpoolID,
		"location": session.LocationName,
	}

	spoolMeta := buildNFCSessionSpoolMeta(ws, session.SpoolID)
	if name, ok := spoolMeta["name"].(string); ok && name != "" {
		success["spool_name"] = name
	}
	if material, ok := spoolMeta["material"].(string); ok && material != "" {
		success["spool_material"] = material
	}
	if brand, ok := spoolMeta["brand"].(string); ok && brand != "" {
		success["spool_brand"] = brand
	}
	if color, ok := spoolMeta["color_hex"].(string); ok && color != "" {
		success["spool_color"] = color
	}
	if weight, ok := spoolMeta["remaining_weight"].(float64); ok && weight > 0 {
		success["spool_weight"] = weight
	}

	if bambu.IsBambuLocation(session.LocationName) {
		success["location_type"] = "ams_slot"
		if idx := strings.Index(session.LocationName, " - "); idx >= 0 {
			success["printer"] = session.LocationName[:idx]
			success["slot"] = strings.TrimSpace(session.LocationName[idx+3:])
		}
	} else if session.IsPrinterLocation && session.PrinterName != "" {
		success["location_type"] = "toolhead"
		success["printer"] = session.PrinterName
		success["toolhead"] = toolheadDisplayName
	} else {
		success["location_type"] = "storage"
	}

	return gin.H{
		"completed": true,
		"success":   success,
	}
}

func buildNFCSessionLocationMeta(ws *WebServer, session *nfc.Session) gin.H {
	if !session.HasLocation {
		return nil
	}

	locationType := "storage"
	displayName := session.LocationName
	meta := gin.H{
		"name":         session.LocationName,
		"display_name": displayName,
		"location_type": locationType,
	}

	if bambu.IsBambuLocation(session.LocationName) {
		meta["location_type"] = "ams_slot"
		if idx := strings.Index(session.LocationName, " - "); idx >= 0 {
			meta["printer_name"] = session.LocationName[:idx]
			meta["display_name"] = strings.TrimSpace(session.LocationName[idx+3:])
		}
		return meta
	}

	if session.IsPrinterLocation && session.PrinterName != "" {
		meta["location_type"] = "toolhead"
		meta["printer_name"] = session.PrinterName
		meta["toolhead_display_name"] = resolveToolheadDisplayName(ws, session)
		if idx := strings.Index(session.LocationName, " - "); idx >= 0 {
			meta["display_name"] = strings.TrimSpace(session.LocationName[idx+3:])
		}
		return meta
	}

	if printerName, toolheadDisplayName, ok := ws.bridge.ParseVirtualToolheadLocation(session.LocationName); ok {
		meta["location_type"] = "toolhead"
		meta["printer_name"] = printerName
		meta["toolhead_display_name"] = toolheadDisplayName
	}

	return meta
}

func redirectNFCScanNoSpoolsForFilament(c *gin.Context, ws *WebServer, filamentID int) {
	filamentName := fmt.Sprintf("Filament %d", filamentID)
	if filament, err := ws.bridge.Spoolman.GetFilament(filamentID); err == nil && filament.Name != "" {
		filamentName = filament.Name
	}

	q := neturl.Values{}
	q.Set("error", "no_spools_for_filament")
	q.Set("filament_name", filamentName)
	redirectNFCScan(c, q)
}

func (ws *WebServer) resolveFilamentScan(sessionID string, filamentID int, printerName string, toolheadID int, locationName string, isPrinterLocation bool) (*nfc.Session, bool, error) {
	spools, err := ws.bridge.Spoolman.GetSpoolsByFilament(filamentID)
	if err != nil {
		return nil, false, err
	}

	if len(spools) == 0 {
		return nil, true, nil
	}

	if len(spools) == 1 {
		session, err := nfc.CreateOrUpdateSession(ws.bridge, sessionID, spools[0].ID, printerName, toolheadID, locationName, isPrinterLocation)
		return session, false, err
	}

	session, err := nfc.SetPendingFilament(ws.bridge, sessionID, filamentID)
	if err != nil {
		return nil, false, err
	}

	if hasLocation := locationName != ""; hasLocation {
		session, err = nfc.CreateOrUpdateSession(ws.bridge, sessionID, 0, printerName, toolheadID, locationName, isPrinterLocation)
	}
	return session, false, err
}

// nfcAssignHandler handles NFC tag scans and redirects to the Next.js scan UI.
func (ws *WebServer) nfcAssignHandler(c *gin.Context) {
	spoolIDStr := c.Query("spool")
	filamentIDStr := c.Query("filament")
	locationStr := c.Query("location")
	clientIP := nfc.ClientIP(c.ClientIP())

	sessionID := nfc.SessionIDForIP(clientIP)

	var spoolID int
	var printerName string
	var toolheadID int
	var err error

	if spoolIDStr != "" {
		spoolID, err = strconv.Atoi(spoolIDStr)
		if err != nil {
			redirectNFCScanError(c, "Invalid spool ID")
			return
		}
	}

	var locationName string
	var isPrinterLocation bool

	if locationStr != "" {
		if bambu.IsBambuLocation(locationStr) {
			tray, parseErr := bambu.ParseLocation(ws.bridge, locationStr)
			if parseErr != nil {
				redirectNFCScanError(c, parseErr.Error())
				return
			}
			if tray == nil {
				redirectNFCScanError(c, "Bambu tray not found for location")
				return
			}
			locationName = locationStr
			isPrinterLocation = true
		} else {
			printerName, toolheadID, locationName, isPrinterLocation, err = nfc.ParseLocationParam(ws.bridge, locationStr)
			if err != nil {
				redirectNFCScanError(c, err.Error())
				return
			}
		}
	}

	var session *nfc.Session

	if filamentIDStr != "" && spoolID == 0 {
		filamentID, parseErr := strconv.Atoi(filamentIDStr)
		if parseErr != nil {
			redirectNFCScanError(c, "Invalid filament ID")
			return
		}

		var noSpools bool
		session, noSpools, err = ws.resolveFilamentScan(sessionID, filamentID, printerName, toolheadID, locationName, isPrinterLocation)
		if err != nil {
			redirectNFCScanError(c, "Failed to resolve filament: "+err.Error())
			return
		}
		if noSpools {
			redirectNFCScanNoSpoolsForFilament(c, ws, filamentID)
			return
		}
	} else {
		session, err = nfc.CreateOrUpdateSession(ws.bridge, sessionID, spoolID, printerName, toolheadID, locationName, isPrinterLocation)
		if err != nil {
			redirectNFCScanError(c, "Failed to create session: "+err.Error())
			return
		}
	}

	if session.IsComplete() {
		if err := ws.completeNFCAssignment(session); err != nil {
			redirectNFCScanError(c, "Assignment failed: "+err.Error())
			return
		}

		ws.BroadcastStatus()

		toolheadDisplayName := resolveToolheadDisplayName(ws, session)

		nfc.DeleteSession(ws.bridge, sessionID)

		redirectNFCScanSuccess(c, ws, session, toolheadDisplayName)
		return
	}

	redirectNFCScanPending(c, session)
}

// normalizeNFCLocationName lowercases and removes spaces for dedup comparisons.
func normalizeNFCLocationName(name string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), " ", "")
}

func buildFilamentNFCURL(baseURL string, filament spoolman.Filament) gin.H {
	url := fmt.Sprintf("%s/api/nfc/assign?filament=%d", baseURL, filament.ID)

	colorHex := filamentColorHex(&filament)
	brand := ""
	if filament.Vendor != nil {
		brand = filament.Vendor.Name
	}

	entry := gin.H{
		"type":           "filament",
		"filament_id":    filament.ID,
		"filament_name":  filament.Name,
		"material":       filament.Material,
		"brand":          brand,
		"color_hex":      colorHex,
		"url":            url,
		"qr_code_base64": "",
	}

	qrCode, err := qrcode.Encode(url, qrcode.Medium, 256)
	if err != nil {
		log.Printf("Error generating QR code for filament %d: %v", filament.ID, err)
		return entry
	}
	entry["qr_code_base64"] = base64.StdEncoding.EncodeToString(qrCode)
	return entry
}

func buildSpoolNFCURL(baseURL string, spool spoolman.Spool) gin.H {
	url := fmt.Sprintf("%s/api/nfc/assign?spool=%d", baseURL, spool.ID)

	colorHex := spoolColorHex(&spool)

	entry := gin.H{
		"type":             "spool",
		"spool_id":         spool.ID,
		"spool_name":       spool.Name,
		"material":         spool.Material,
		"brand":            spool.Brand,
		"color_hex":        colorHex,
		"remaining_weight": spool.RemainingWeight,
		"url":              url,
		"qr_code_base64":   "",
	}
	if spool.Filament != nil {
		entry["filament_id"] = spool.Filament.ID
		entry["filament_name"] = spool.Filament.Name
	}

	qrCode, err := qrcode.Encode(url, qrcode.Medium, 256)
	if err != nil {
		log.Printf("Error generating QR code for spool %d: %v", spool.ID, err)
		return entry
	}
	entry["qr_code_base64"] = base64.StdEncoding.EncodeToString(qrCode)
	return entry
}

func buildSpoolmanLocationNFCEntry(baseURL string, location spoolman.Location, bridge *core.FilamentBridge) gin.H {
	locationParam := location.Name
	nfcURL := fmt.Sprintf("%s/api/nfc/assign?location=%s", baseURL, neturl.QueryEscape(locationParam))

	locationType := "storage"
	entry := gin.H{
		"type":          "location",
		"location_type": locationType,
		"location_name": location.Name,
		"display_name":  location.Name,
		"url":           nfcURL,
		"is_local_only": false,
	}

	if printerName, toolheadDisplayName, ok := bridge.ParseVirtualToolheadLocation(location.Name); ok {
		entry["location_type"] = "toolhead"
		entry["printer_name"] = printerName
		entry["toolhead_display_name"] = toolheadDisplayName
	}

	qrCode, err := qrcode.Encode(nfcURL, qrcode.Medium, 256)
	if err != nil {
		log.Printf("Error generating QR code for Spoolman location %s: %v", locationParam, err)
		entry["qr_code_base64"] = ""
		return entry
	}
	entry["qr_code_base64"] = base64.StdEncoding.EncodeToString(qrCode)
	return entry
}

// nfcUrlsHandler returns all available NFC URLs with QR codes.
func (ws *WebServer) nfcUrlsHandler(c *gin.Context) {
	var urls []gin.H

	baseURL := nfcBaseURL(ws.bridge, c)

	spools, err := ws.bridge.Spoolman.GetAllSpools()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	for _, spool := range spools {
		urls = append(urls, buildSpoolNFCURL(baseURL, spool))
	}

	filaments, err := ws.bridge.Spoolman.GetAllFilaments()
	if err != nil {
		log.Printf("Warning: Failed to get filaments for NFC URLs: %v", err)
		filaments = []spoolman.Filament{}
	}
	for _, filament := range filaments {
		urls = append(urls, buildFilamentNFCURL(baseURL, filament))
	}

	bambuLocationNames := make(map[string]struct{})
	if bambuURLs, err := bambu.GenerateNFCURLs(ws.bridge, baseURL); err == nil {
		for _, entry := range bambuURLs {
			urls = append(urls, entry)
			if displayName, ok := entry["display_name"].(string); ok {
				bambuLocationNames[normalizeNFCLocationName(displayName)] = struct{}{}
			}
		}
	}

	toolheadLocationNames := make(map[string]struct{})
	if toolheadURLs, err := core.GenerateToolheadNFCURLs(ws.bridge, baseURL); err == nil {
		for _, entry := range toolheadURLs {
			urls = append(urls, entry)
			if displayName, ok := entry["display_name"].(string); ok {
				toolheadLocationNames[normalizeNFCLocationName(displayName)] = struct{}{}
			}
		}
	} else {
		log.Printf("Warning: Failed to generate toolhead NFC URLs: %v", err)
	}

	spoolmanLocations, err := ws.bridge.Spoolman.GetLocations()
	if err != nil {
		log.Printf("Warning: Failed to get Spoolman locations: %v", err)
		spoolmanLocations = []spoolman.Location{}
	}

	for _, location := range spoolmanLocations {
		if location.Archived {
			continue
		}

		if strings.TrimSpace(location.Name) == "" {
			continue
		}

		if bambu.IsBambuLocation(location.Name) {
			continue
		}
		if _, exists := bambuLocationNames[normalizeNFCLocationName(location.Name)]; exists {
			continue
		}
		if _, exists := toolheadLocationNames[normalizeNFCLocationName(location.Name)]; exists {
			continue
		}

		urls = append(urls, buildSpoolmanLocationNFCEntry(baseURL, location, ws.bridge))
	}

	sort.Slice(urls, func(i, j int) bool {
		typeI := urls[i]["type"].(string)
		typeJ := urls[j]["type"].(string)

		if typeI != typeJ {
			typeOrder := map[string]int{
				"spool":     0,
				"filament":  1,
				"location":  2,
			}
			return typeOrder[typeI] < typeOrder[typeJ]
		}

		if typeI == "filament" {
			nameI := urls[i]["filament_name"].(string)
			nameJ := urls[j]["filament_name"].(string)
			return strings.ToLower(nameI) < strings.ToLower(nameJ)
		}

		if typeI == "location" {
			displayNameI := urls[i]["display_name"].(string)
			displayNameJ := urls[j]["display_name"].(string)
			return strings.ToLower(displayNameI) < strings.ToLower(displayNameJ)
		}

		materialI := urls[i]["material"].(string)
		materialJ := urls[j]["material"].(string)
		brandI := urls[i]["brand"].(string)
		brandJ := urls[j]["brand"].(string)
		nameI := urls[i]["spool_name"].(string)
		nameJ := urls[j]["spool_name"].(string)

		displayNameI := fmt.Sprintf("%s - %s - %s", materialI, brandI, nameI)
		displayNameJ := fmt.Sprintf("%s - %s - %s", materialJ, brandJ, nameJ)

		if displayNameI != displayNameJ {
			return displayNameI < displayNameJ
		}

		weightI := urls[i]["remaining_weight"].(float64)
		weightJ := urls[j]["remaining_weight"].(float64)
		return weightI < weightJ
	})

	spoolmanURL := ws.bridge.Spoolman.GetBaseURL()

	c.JSON(http.StatusOK, gin.H{
		"urls":         urls,
		"spoolman_url": spoolmanURL,
	})
}

// nfcSessionStatusHandler returns the current session status with enriched metadata.
func (ws *WebServer) nfcSessionStatusHandler(c *gin.Context) {
	sessionID := nfcSessionIDFromRequest(c)

	session, err := nfc.GetSession(ws.bridge, sessionID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"active": false,
		})
		return
	}

	resp := gin.H{
		"active":               true,
		"session_id":           session.SessionID,
		"has_spool":            session.HasSpool,
		"has_pending_filament": session.HasPendingFilament,
		"has_location":         session.HasLocation,
		"spool_id":             session.SpoolID,
		"pending_filament_id":  session.PendingFilamentID,
		"printer_name":         session.PrinterName,
		"toolhead_id":          session.ToolheadID,
		"location_name":        session.LocationName,
		"is_printer_location":  session.IsPrinterLocation,
		"expires_at":           session.ExpiresAt,
	}

	if session.HasSpool {
		resp["spool"] = buildNFCSessionSpoolMeta(ws, session.SpoolID)
	}
	if session.HasPendingFilament {
		resp["pending_filament"] = buildNFCSessionPendingFilamentMeta(ws, session)
	}
	if session.HasLocation {
		resp["location"] = buildNFCSessionLocationMeta(ws, session)
	}

	c.JSON(http.StatusOK, resp)
}

func (ws *WebServer) nfcSelectSpoolHandler(c *gin.Context) {
	var body struct {
		SpoolID   int    `json:"spool_id"`
		SessionID string `json:"session_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.SpoolID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid spool ID"})
		return
	}

	sessionID := strings.TrimSpace(body.SessionID)
	if sessionID == "" {
		sessionID = nfcSessionIDFromRequest(c)
	}

	session, err := nfc.GetSession(ws.bridge, sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No active NFC session"})
		return
	}

	if !session.HasPendingFilament {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No pending filament selection"})
		return
	}

	spools, err := ws.bridge.Spoolman.GetSpoolsByFilament(session.PendingFilamentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	validSpool := false
	for _, spool := range spools {
		if spool.ID == body.SpoolID {
			validSpool = true
			break
		}
	}
	if !validSpool {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Selected spool does not belong to the pending filament"})
		return
	}

	session, err = nfc.SelectSpool(ws.bridge, sessionID, body.SpoolID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if session.IsComplete() {
		if err := ws.completeNFCAssignment(session); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Assignment failed: " + err.Error()})
			return
		}

		ws.BroadcastStatus()
		nfc.DeleteSession(ws.bridge, sessionID)
		c.JSON(http.StatusOK, buildNFCSelectSpoolSuccessPayload(ws, session))
		return
	}

	sessionResp := gin.H{
		"active":               true,
		"session_id":           session.SessionID,
		"has_spool":            session.HasSpool,
		"has_pending_filament": session.HasPendingFilament,
		"has_location":         session.HasLocation,
	}
	if session.HasSpool {
		sessionResp["spool"] = buildNFCSessionSpoolMeta(ws, session.SpoolID)
	}
	if session.HasLocation {
		sessionResp["location"] = buildNFCSessionLocationMeta(ws, session)
	}

	c.JSON(http.StatusOK, gin.H{
		"completed": false,
		"session":   sessionResp,
	})
}
