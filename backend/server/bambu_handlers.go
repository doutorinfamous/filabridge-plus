package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"filabridge/bambu"
	"filabridge/core"
)

// registerBambuRoutes registers Bambu/HA API routes on the router group.
func (ws *WebServer) registerBambuRoutes(api *gin.RouterGroup) {
	api.GET("/webhook", ws.bambuWebhookHealthHandler)
	api.POST("/webhook", ws.bambuWebhookHandler)
	api.GET("/ha/test", ws.haTestHandler)
	api.POST("/ha/test", ws.haTestHandler)
	api.GET("/ha/config", ws.getHAConfigHandler)
	api.POST("/ha/config", ws.updateHAConfigHandler)
	api.GET("/ha/printers", ws.haPrintersHandler)
	api.POST("/ha/printers", ws.haRegisterPrinterHandler)
	api.DELETE("/ha/printers/:id", ws.haRemovePrinterHandler)
	api.GET("/ha/automations/:id", ws.haAutomationsHandler)
	api.GET("/ha/validate/:id", ws.haValidateHandler)
	api.POST("/trays/assign", ws.trayAssignHandler)
}

func (ws *WebServer) bambuWebhookHealthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "FilaBridge+ webhook endpoint — use POST with JSON body",
		"events":  []string{"spool_usage", "tray_change", "print_started", "print_finished"},
	})
}

func (ws *WebServer) bambuWebhookHandler(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON payload"})
		return
	}
	body = bytes.TrimPrefix(body, []byte{0xEF, 0xBB, 0xBF})

	var payload bambu.WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON payload"})
		return
	}

	ha, _ := bambu.NewHAClientFromConfig(ws.bridge)
	result := bambu.ProcessWebhook(ws.bridge, payload, ha)
	c.JSON(http.StatusOK, result)
}

func (ws *WebServer) haTestHandler(c *gin.Context) {
	var req struct {
		HAURL   string `json:"ha_url"`
		HAToken string `json:"ha_token"`
	}
	if c.Request.Method == http.MethodPost {
		_ = c.ShouldBindJSON(&req)
	}

	url := strings.TrimSpace(req.HAURL)
	token := strings.TrimSpace(req.HAToken)
	if url == "" {
		url, _ = bambu.GetHAURL(ws.bridge)
	}
	if token == "" {
		token, _ = bambu.GetHAToken(ws.bridge)
	}

	ha, err := bambu.NewHAClientFromCredentials(url, token)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if err := ha.TestConnection(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (ws *WebServer) getHAConfigHandler(c *gin.Context) {
	url, _ := bambu.GetHAURL(ws.bridge)
	publicURL, _ := bambu.GetFilabridgePublicURL(ws.bridge)
	token, _ := bambu.GetHAToken(ws.bridge)
	c.JSON(http.StatusOK, gin.H{
		"ha_url":                url,
		"ha_token_set":          token != "",
		"filabridge_public_url": publicURL,
	})
}

func (ws *WebServer) updateHAConfigHandler(c *gin.Context) {
	var req struct {
		HAURL               string `json:"ha_url"`
		HAToken             string `json:"ha_token"`
		FilabridgePublicURL string `json:"filabridge_public_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.HAURL != "" {
		if err := ws.bridge.SetConfigValue(core.ConfigKeyHAURL, strings.TrimSpace(req.HAURL)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if req.HAToken != "" {
		if err := ws.bridge.SetConfigValue(core.ConfigKeyHAToken, strings.TrimSpace(req.HAToken)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if req.FilabridgePublicURL != "" {
		if err := ws.bridge.SetConfigValue(core.ConfigKeyFilabridgePublicURL, strings.TrimRight(strings.TrimSpace(req.FilabridgePublicURL), "/")); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (ws *WebServer) haPrintersHandler(c *gin.Context) {
	ha, err := bambu.NewHAClientFromConfig(ws.bridge)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	printers, err := bambu.DiscoverPrinters(ha)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	printers, _ = bambu.EnrichPrintersWithAssignments(ws.bridge, printers)
	c.JSON(http.StatusOK, printers)
}

func (ws *WebServer) haRegisterPrinterHandler(c *gin.Context) {
	var printer bambu.Printer
	if err := c.ShouldBindJSON(&printer); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	printerID, err := bambu.RegisterPrinter(ws.bridge, printer)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ws.BroadcastStatus()
	c.JSON(http.StatusOK, gin.H{"printer_id": printerID, "success": true})
}

func (ws *WebServer) haRemovePrinterHandler(c *gin.Context) {
	printerID := c.Param("id")
	if err := bambu.RemovePrinter(ws.bridge, printerID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ws.BroadcastStatus()
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (ws *WebServer) haAutomationsHandler(c *gin.Context) {
	printerID := c.Param("id")
	configs, err := ws.bridge.GetAllPrinterConfigs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	cfg, ok := configs[printerID]
	if !ok || cfg.Driver != core.DriverBambuHA {
		c.JSON(http.StatusNotFound, gin.H{"error": "bambu printer not found"})
		return
	}

	ha, err := bambu.NewHAClientFromConfig(ws.bridge)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	discovered, err := bambu.DiscoverPrinters(ha)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var printer *bambu.Printer
	for i := range discovered {
		if bambu.NormalizePrefix(discovered[i].Prefix) == bambu.NormalizePrefix(cfg.HAPrefix) || discovered[i].DeviceID == cfg.HADeviceID {
			printer = &discovered[i]
			break
		}
	}
	if printer == nil {
		trays, _ := bambu.GetTraysForPrinter(ws.bridge, printerID)
		printer = &bambu.Printer{
			Prefix:              cfg.HAPrefix,
			Name:                cfg.Name,
			PrintWeightEntity:   "",
			PrintProgressEntity: "",
			CurrentStageEntity:  cfg.HAPrefix,
		}
		amsMap := make(map[int]*bambu.AMS)
		for _, t := range trays {
			if t.IsExternal {
				printer.ExternalSpools = append(printer.ExternalSpools, t)
				continue
			}
			ams, ok := amsMap[t.AMSNumber]
			if !ok {
				ams = &bambu.AMS{AMSNumber: t.AMSNumber, Name: fmt.Sprintf("AMS %d", t.AMSNumber)}
				amsMap[t.AMSNumber] = ams
			}
			ams.Trays = append(ams.Trays, t)
		}
		for _, ams := range amsMap {
			printer.AMSUnits = append(printer.AMSUnits, *ams)
		}
	}

	webhookURL, _ := bambu.GetFilabridgePublicURL(ws.bridge)
	yaml := bambu.GenerateHAPackage(*printer, webhookURL+"/api/webhook")
	c.JSON(http.StatusOK, gin.H{
		"yaml":        yaml,
		"webhook_url": webhookURL + "/api/webhook",
		"filename":    "filabridge_" + bambu.NormalizePrefix(cfg.HAPrefix) + ".yaml",
	})
}

func (ws *WebServer) haValidateHandler(c *gin.Context) {
	printerID := c.Param("id")
	configs, err := ws.bridge.GetAllPrinterConfigs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	cfg, ok := configs[printerID]
	if !ok || cfg.Driver != core.DriverBambuHA {
		c.JSON(http.StatusNotFound, gin.H{"error": "bambu printer not found"})
		return
	}

	ha, err := bambu.NewHAClientFromConfig(ws.bridge)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	states, err := ha.GetStates()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	result := bambu.ValidateHAEntities(cfg.HAPrefix, states)
	c.JSON(http.StatusOK, result)
}

func (ws *WebServer) trayAssignHandler(c *gin.Context) {
	var req bambu.TrayAssignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.TrayUniqueID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tray_unique_id is required"})
		return
	}

	if req.SpoolID <= 0 {
		if err := bambu.UnassignTray(ws.bridge, req.TrayUniqueID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		ws.BroadcastStatus()
		c.JSON(http.StatusOK, gin.H{"success": true, "action": "unassigned"})
		return
	}

	displayName := req.TrayUniqueID
	if tray, err := bambu.FindTrayByUniqueID(ws.bridge, req.TrayUniqueID); err == nil && tray != nil {
		displayName = tray.DisplayName
	}

	if err := bambu.AssignSpoolToTray(ws.bridge, req.SpoolID, req.TrayUniqueID, displayName); err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "already assigned") {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	ws.BroadcastStatus()
	c.JSON(http.StatusOK, gin.H{"success": true})
}
