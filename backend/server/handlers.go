package server

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"filabridge/bambu"
	"filabridge/core"
	"filabridge/snapmaker"
)

// statusHandler returns current status as JSON.
func (ws *WebServer) statusHandler(c *gin.Context) {
	status, err := ws.BuildStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, status)
}

// spoolsHandler returns all spools as JSON.
func (ws *WebServer) spoolsHandler(c *gin.Context) {
	spools, err := ws.bridge.Spoolman.GetAllSpools()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, spools)
}

// filamentsHandler returns all filament types as JSON.
func (ws *WebServer) filamentsHandler(c *gin.Context) {
	filaments, err := ws.bridge.Spoolman.GetAllFilaments()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, filaments)
}

// validatePrinterConfig validates printer configuration input.
func validatePrinterConfig(config core.PrinterConfig) error {
	if config.Name == "" {
		return fmt.Errorf("printer name is required")
	}
	if config.Driver == core.DriverBambuHA {
		return nil
	}
	if config.IPAddress == "" {
		return fmt.Errorf("address is required")
	}
	if config.Toolheads < 1 {
		return fmt.Errorf("toolheads must be at least 1")
	}
	if config.Toolheads > 10 {
		return fmt.Errorf("toolheads cannot exceed 10")
	}
	return nil
}

// validateAddress validates hostname or IP address format.
func validateAddress(address string) error {
	if address == "" {
		return fmt.Errorf("address cannot be empty")
	}
	if len(address) < 1 || len(address) > 253 {
		return fmt.Errorf("invalid address format")
	}
	for _, char := range address {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '.' || char == '-' || char == '_' ||
			char == ':' || char == '[' || char == ']') {
			return fmt.Errorf("invalid address format: contains invalid characters")
		}
	}
	return nil
}

// mapToolheadHandler maps a spool to a toolhead.
func (ws *WebServer) mapToolheadHandler(c *gin.Context) {
	var req struct {
		PrinterName string `json:"printer_name" binding:"required"`
		ToolheadID  int    `json:"toolhead_id"`
		SpoolID     int    `json:"spool_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	if req.PrinterName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing required parameters"})
		return
	}

	if req.ToolheadID < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Toolhead ID must be non-negative"})
		return
	}

	// Handle unmapping (SpoolID = 0) or mapping (SpoolID > 0)
	if req.SpoolID == 0 {
		printerID, err := ws.bridge.FindPrinterIDByName(req.PrinterName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if printerID == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Printer not found"})
			return
		}

		previousSpoolID, err := ws.bridge.GetToolheadMapping(printerID, req.ToolheadID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if err := ws.bridge.UnmapToolhead(printerID, req.ToolheadID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if previousSpoolID > 0 {
			ws.bridge.TryAutoAssignSpoolToDefaultStorage(previousSpoolID)
		}

		ws.BroadcastStatus()
		c.JSON(http.StatusOK, gin.H{"message": "Toolhead unmapped successfully"})
	} else {
		// Map the spool to the toolhead and sync location to Spoolman (same as NFC/QR)
		if err := ws.bridge.AssignSpoolToLocation(req.SpoolID, req.PrinterName, req.ToolheadID, "", true); err != nil {
			if strings.Contains(err.Error(), "is already assigned to") {
				c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			}
			return
		}
		ws.BroadcastStatus()
		c.JSON(http.StatusOK, gin.H{"message": "Toolhead mapped successfully"})
	}
}

// availableSpoolsHandler returns spools available for assignment to a specific toolhead.
func (ws *WebServer) availableSpoolsHandler(c *gin.Context) {
	printerName := c.Query("printer_name")
	toolheadIDStr := c.Query("toolhead_id")
	trayUniqueID := c.Query("tray_unique_id")

	var exclude core.ExcludeAssignment
	switch {
	case trayUniqueID != "":
		exclude.TrayUniqueID = trayUniqueID
	case printerName != "" && toolheadIDStr != "":
		toolheadID, err := strconv.Atoi(toolheadIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid toolhead_id"})
			return
		}
		printerID, err := ws.bridge.FindPrinterIDByName(printerName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		exclude.PrinterID = printerID
		exclude.ToolheadID = toolheadID
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "provide printer_name+toolhead_id or tray_unique_id"})
		return
	}

	availableSpools, err := ws.bridge.GetAvailableSpools(exclude)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"spools": availableSpools})
}

// getConfigHandler returns current configuration.
func (ws *WebServer) getConfigHandler(c *gin.Context) {
	config, err := ws.bridge.GetAllConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, config)
}

// updateConfigHandler updates configuration.
func (ws *WebServer) updateConfigHandler(c *gin.Context) {
	var config map[string]string
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	for key, value := range config {
		if err := ws.bridge.SetConfigValue(key, value); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	newConfig, err := core.LoadConfig(ws.bridge)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := ws.bridge.UpdateConfig(newConfig); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Configuration updated successfully"})
}

// getAutoAssignPreviousSpoolHandler returns current auto-assign previous spool settings.
func (ws *WebServer) getAutoAssignPreviousSpoolHandler(c *gin.Context) {
	enabled, err := ws.bridge.GetAutoAssignPreviousSpoolEnabled()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	location, err := ws.bridge.GetAutoAssignPreviousSpoolLocation()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"enabled":  enabled,
		"location": location,
	})
}

// updateAutoAssignPreviousSpoolHandler updates auto-assign previous spool settings.
func (ws *WebServer) updateAutoAssignPreviousSpoolHandler(c *gin.Context) {
	var req struct {
		Enabled  bool   `json:"enabled" binding:"required"`
		Location string `json:"location"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON or missing 'enabled' field"})
		return
	}

	if err := ws.bridge.SetAutoAssignPreviousSpoolEnabled(req.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := ws.bridge.SetAutoAssignPreviousSpoolLocation(req.Location); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Auto-assign previous spool settings updated successfully"})
}

// getPrintersHandler returns all configured printers.
func (ws *WebServer) getPrintersHandler(c *gin.Context) {
	printerConfigs, err := ws.bridge.GetAllPrinterConfigs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make(map[string]interface{})
	for printerID, printerConfig := range printerConfigs {
		printerData := map[string]interface{}{
			"name":         printerConfig.Name,
			"model":        printerConfig.Model,
			"driver":       printerConfig.Driver,
			"ip_address":   printerConfig.IPAddress,
			"api_key":      printerConfig.APIKey,
			"toolheads":    printerConfig.Toolheads,
			"ha_prefix":    printerConfig.HAPrefix,
			"ha_device_id": printerConfig.HADeviceID,
		}

		toolheadNames, err := ws.bridge.GetAllToolheadNames(printerID)
		if err == nil {
			toolheadNamesMap := make(map[int]string)
			for toolheadID := 0; toolheadID < printerConfig.Toolheads; toolheadID++ {
				if name, exists := toolheadNames[toolheadID]; exists {
					toolheadNamesMap[toolheadID] = name
				} else {
					toolheadNamesMap[toolheadID] = core.DefaultToolheadDisplayName(toolheadID)
				}
			}
			printerData["toolhead_names"] = toolheadNamesMap
		}

		result[printerID] = printerData
	}

	c.JSON(http.StatusOK, gin.H{"printers": result})
}

// addPrinterHandler adds a new printer configuration.
func (ws *WebServer) addPrinterHandler(c *gin.Context) {
	ws.operationMutex.Lock()
	defer ws.operationMutex.Unlock()

	var printerConfig core.PrinterConfig
	if err := c.ShouldBindJSON(&printerConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := validatePrinterConfig(printerConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if printerConfig.Driver != core.DriverBambuHA {
		if err := validateAddress(printerConfig.IPAddress); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	printerID := fmt.Sprintf("printer_%d_%d", time.Now().UnixNano(), time.Now().Nanosecond()%1000)

	if err := ws.bridge.SavePrinterConfig(printerID, printerConfig); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := ws.reloadBridgeConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reload configuration"})
		return
	}

	ws.bridge.EnsurePrinterToolheadLocationsInSpoolman(printerID, printerConfig.Name, 0, printerConfig.Toolheads)

	c.JSON(http.StatusOK, gin.H{"message": "Printer added successfully", "printer_id": printerID})
}

// updatePrinterHandler updates an existing printer configuration.
func (ws *WebServer) updatePrinterHandler(c *gin.Context) {
	ws.operationMutex.Lock()
	defer ws.operationMutex.Unlock()

	printerID := c.Param("id")

	oldPrinterConfigs, err := ws.bridge.GetAllPrinterConfigs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	oldPrinterConfig, hadOldConfig := oldPrinterConfigs[printerID]

	var printerConfig core.PrinterConfig
	if err := c.ShouldBindJSON(&printerConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := validatePrinterConfig(printerConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := validateAddress(printerConfig.IPAddress); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Auto-detect model if model is currently "Unknown" or missing
	if printerConfig.Model == "" || printerConfig.Model == snapmaker.ModelUnknown {
		log.Printf("🔍 [Auto-Detection] Detecting model for printer %s (IP: %s)", printerID, printerConfig.IPAddress)

		client := snapmaker.NewMoonrakerClient(printerConfig.IPAddress, printerConfig.APIKey, 10, 60)

		printerInfo, err := client.GetPrinterInfo()
		if err != nil {
			log.Printf("⚠️ [Auto-Detection] Failed to detect model for %s: %v (keeping current model: %s)",
				printerConfig.IPAddress, err, printerConfig.Model)
		} else {
			detectedModel := snapmaker.DetectPrinterModel(printerInfo.Hostname)

			if detectedModel == snapmaker.ModelUnknown {
				detectedModel = snapmaker.ModelSnapmakerU1
			}

			log.Printf("✅ [Auto-Detection] Detected model for %s: '%s' -> %s",
				printerConfig.IPAddress, printerInfo.Hostname, detectedModel)
			printerConfig.Model = detectedModel
		}
	}

	if err := ws.bridge.SavePrinterConfig(printerID, printerConfig); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := ws.reloadBridgeConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reload configuration"})
		return
	}

	if hadOldConfig && printerConfig.Toolheads > oldPrinterConfig.Toolheads {
		ws.bridge.EnsurePrinterToolheadLocationsInSpoolman(
			printerID,
			printerConfig.Name,
			oldPrinterConfig.Toolheads,
			printerConfig.Toolheads,
		)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Printer updated successfully"})
}

// deletePrinterHandler deletes a printer configuration.
func (ws *WebServer) deletePrinterHandler(c *gin.Context) {
	ws.operationMutex.Lock()
	defer ws.operationMutex.Unlock()

	printerID := c.Param("id")

	printerConfigs, err := ws.bridge.GetAllPrinterConfigs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if cfg, ok := printerConfigs[printerID]; ok && cfg.Driver == core.DriverBambuHA {
		if err := bambu.RemovePrinter(ws.bridge, printerID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else if err := ws.bridge.DeletePrinterConfig(printerID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := ws.reloadBridgeConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reload configuration"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Printer deleted successfully"})
}

// getToolheadNamesHandler returns all toolhead names for a printer.
func (ws *WebServer) getToolheadNamesHandler(c *gin.Context) {
	printerID := c.Param("id")

	printerConfigs, err := ws.bridge.GetAllPrinterConfigs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	printerConfig, exists := printerConfigs[printerID]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Printer not found"})
		return
	}

	toolheadNames, err := ws.bridge.GetAllToolheadNames(printerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make(map[int]string)
	for toolheadID := 0; toolheadID < printerConfig.Toolheads; toolheadID++ {
		if name, exists := toolheadNames[toolheadID]; exists {
			result[toolheadID] = name
		} else {
			result[toolheadID] = core.DefaultToolheadDisplayName(toolheadID)
		}
	}

	c.JSON(http.StatusOK, gin.H{"toolhead_names": result})
}

// updateToolheadNameHandler updates a toolhead's display name.
func (ws *WebServer) updateToolheadNameHandler(c *gin.Context) {
	printerID := c.Param("id")
	toolheadIDStr := c.Param("toolhead_id")

	toolheadID, err := strconv.Atoi(toolheadIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid toolhead ID"})
		return
	}

	printerConfigs, err := ws.bridge.GetAllPrinterConfigs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	printerConfig, exists := printerConfigs[printerID]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Printer not found"})
		return
	}

	if toolheadID < 0 || toolheadID >= printerConfig.Toolheads {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Toolhead ID must be between 0 and %d", printerConfig.Toolheads-1)})
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON or missing 'name' field"})
		return
	}

	if err := ws.bridge.SetToolheadName(printerID, toolheadID, req.Name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Toolhead name updated successfully"})
}

// detectPrinterHandler detects printer model from Snapmaker U1 Moonraker API.
func (ws *WebServer) detectPrinterHandler(c *gin.Context) {
	var req struct {
		IPAddress string `json:"ip_address" binding:"required"`
		APIKey    string `json:"api_key"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	if err := validateAddress(req.IPAddress); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("🔍 [Detection] Starting printer model detection for IP: %s", req.IPAddress)

	client := snapmaker.NewMoonrakerClient(req.IPAddress, req.APIKey, 10, 60)

	printerInfo, err := client.GetPrinterInfo()
	if err != nil {
		log.Printf("❌ [Detection] Failed to get printer info from %s: %v", req.IPAddress, err)
		// Allow adding printers even if they're offline
		c.JSON(http.StatusOK, gin.H{
			"model":    snapmaker.ModelUnknown,
			"hostname": "Unknown",
			"detected": false,
			"warning":  "Could not connect to printer. You can still add it manually.",
		})
		return
	}

	log.Printf("📥 [Detection] Received printer info: hostname='%s'", printerInfo.Hostname)

	model := snapmaker.DetectPrinterModel(printerInfo.Hostname)
	if model == snapmaker.ModelUnknown {
		model = snapmaker.ModelSnapmakerU1
	}

	c.JSON(http.StatusOK, gin.H{
		"model":    model,
		"hostname": printerInfo.Hostname,
		"detected": true,
	})
}

// testSpoolmanConnectionHandler tests the connection to Spoolman.
func (ws *WebServer) testSpoolmanConnectionHandler(c *gin.Context) {
	if err := ws.bridge.Spoolman.TestConnection(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "connected": false})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Connection successful", "connected": true})
}

// debugSpoolmanHandler provides detailed debug information about Spoolman data.
func (ws *WebServer) debugSpoolmanHandler(c *gin.Context) {
	spools, err := ws.bridge.Spoolman.GetAllSpools()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	debugInfo := gin.H{
		"spool_count": len(spools),
		"spools":      spools,
		"raw_data":    make([]gin.H, len(spools)),
	}

	for i, spool := range spools {
		debugInfo["raw_data"].([]gin.H)[i] = gin.H{
			"id":               spool.ID,
			"name":             spool.Name,
			"brand":            spool.Brand,
			"material":         spool.Material,
			"color":            spool.Filament.ColorHex,
			"remaining_length": spool.RemainingLength,
			"name_empty":       spool.Name == "",
			"brand_empty":      spool.Brand == "",
			"material_empty":   spool.Material == "",
			"color_empty":      spool.Filament.ColorHex == "",
		}
	}

	c.JSON(http.StatusOK, debugInfo)
}

// testPrintCompleteHandler simulates a print completion for testing.
func (ws *WebServer) testPrintCompleteHandler(c *gin.Context) {
	var request struct {
		PrinterName   string          `json:"printer_name" binding:"required"`
		JobName       string          `json:"job_name"`
		FilamentUsage map[int]float64 `json:"filament_usage"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if request.JobName == "" {
		request.JobName = "Test Print Job"
	}

	if len(request.FilamentUsage) == 0 {
		request.FilamentUsage = map[int]float64{
			0: 10.0, // 10g for toolhead 0
		}
	}

	// Get printer config - first try by name, then by ID
	var printerID string
	var found bool

	for id, printerConfig := range ws.bridge.Config.Printers {
		if printerConfig.Name == request.PrinterName {
			printerID = id
			found = true
			break
		}
	}

	if !found {
		if _, ok := ws.bridge.Config.Printers[request.PrinterName]; ok {
			printerID = request.PrinterName
			found = true
		}
	}

	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "Printer not found"})
		return
	}

	if err := ws.bridge.ProcessFilamentUsage(printerID, request.FilamentUsage, request.JobName); err != nil {
		log.Printf("Error processing filament usage: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        "Print completion simulated successfully",
		"printer":        request.PrinterName,
		"job":            request.JobName,
		"filament_usage": request.FilamentUsage,
	})
}

// getPrintErrorsHandler returns all unacknowledged print errors.
func (ws *WebServer) getPrintErrorsHandler(c *gin.Context) {
	errors := ws.bridge.GetPrintErrors()
	c.JSON(http.StatusOK, gin.H{
		"errors": errors,
	})
}

// acknowledgePrintErrorHandler acknowledges a print error.
func (ws *WebServer) acknowledgePrintErrorHandler(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in acknowledgePrintErrorHandler: %v", r)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		}
	}()

	errorID := c.Param("id")
	if errorID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Error ID is required"})
		return
	}

	if err := ws.bridge.AcknowledgePrintError(errorID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Error acknowledged"})
}

// reloadBridgeConfig reloads the bridge configuration after changes.
func (ws *WebServer) reloadBridgeConfig() error {
	if err := ws.bridge.ReloadConfig(); err != nil {
		return fmt.Errorf("failed to reload configuration: %w", err)
	}
	return nil
}

// Location Management Handlers

// getLocationsHandler returns only Spoolman locations (no virtual printer toolheads).
func (ws *WebServer) getLocationsHandler(c *gin.Context) {
	spoolmanLocations, err := ws.bridge.Spoolman.GetLocations()
	if err != nil {
		log.Printf("Warning: Failed to get Spoolman locations: %v", err)
		spoolmanLocations = nil
	}

	var allLocations []gin.H
	for _, loc := range spoolmanLocations {
		if loc.Archived {
			continue
		}

		if strings.TrimSpace(loc.Name) == "" {
			continue
		}

		allLocations = append(allLocations, gin.H{
			"name":       loc.Name,
			"type":       "storage",
			"is_virtual": false,
		})
	}

	spoolmanURL := ws.bridge.Spoolman.GetBaseURL()

	c.JSON(http.StatusOK, gin.H{
		"locations":    allLocations,
		"spoolman_url": spoolmanURL,
	})
}

// getLocationStatusHandler returns detailed status information for a specific location.
func (ws *WebServer) getLocationStatusHandler(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Location name is required"})
		return
	}

	location, err := ws.bridge.Spoolman.FindLocationByName(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if location == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Location not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"name":     location.Name,
		"id":       location.ID,
		"comment":  location.Comment,
		"archived": location.Archived,
	})
}

// createLocationHandler creates a new location in Spoolman.
func (ws *WebServer) createLocationHandler(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Type        string `json:"type"`
		PrinterName string `json:"printer_name,omitempty"`
		ToolheadID  int    `json:"toolhead_id,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("createLocationHandler: bad request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("createLocationHandler: creating location name='%s' in Spoolman", req.Name)
	location, err := ws.bridge.Spoolman.GetOrCreateLocation(req.Name)
	if err != nil {
		log.Printf("createLocationHandler: failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"name":     location.Name,
		"id":       location.ID,
		"comment":  location.Comment,
		"archived": location.Archived,
	})
}

// updateLocationHandler updates a location in Spoolman.
func (ws *WebServer) updateLocationHandler(c *gin.Context) {
	oldName := c.Param("name")
	if oldName == "" {
		log.Printf("updateLocationHandler: missing location name")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Location name is required"})
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("updateLocationHandler: bad request for name='%s': %v", oldName, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("updateLocationHandler: renaming '%s' to '%s' in Spoolman", oldName, req.Name)
	if err := ws.bridge.Spoolman.UpdateLocationByName(oldName, req.Name); err != nil {
		log.Printf("updateLocationHandler: failed for name='%s': %v", oldName, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	location, err := ws.bridge.Spoolman.FindLocationByName(req.Name)
	if err != nil {
		log.Printf("Warning: Could not get updated location '%s': %v", req.Name, err)
		c.JSON(http.StatusOK, gin.H{"message": "Location updated successfully"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Location updated successfully",
		"location": gin.H{
			"name":     location.Name,
			"id":       location.ID,
			"comment":  location.Comment,
			"archived": location.Archived,
		},
	})
}

// deleteLocationHandler archives a location in Spoolman (locations are archived, not deleted).
func (ws *WebServer) deleteLocationHandler(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		log.Printf("deleteLocationHandler: missing location name")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Location name is required"})
		return
	}

	location, err := ws.bridge.Spoolman.FindLocationByName(name)
	if err != nil {
		log.Printf("deleteLocationHandler: error finding location '%s': %v", name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if location == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Location not found"})
		return
	}

	log.Printf("deleteLocationHandler: archiving location '%s' (ID: %d)", name, location.ID)
	if err := ws.bridge.Spoolman.ArchiveLocation(location.ID); err != nil {
		log.Printf("deleteLocationHandler: failed to archive location '%s': %v", name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to archive location"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Location archived successfully"})
}
