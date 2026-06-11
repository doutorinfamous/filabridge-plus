package server

import (
	"encoding/base64"
	"fmt"
	"html/template"
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

// Minimal server-rendered pages for the NFC scan flow. Physical NFC tags point
// directly at /api/nfc/assign, so this endpoint must answer with browsable HTML
// even though the rest of the UI lives in the Next.js app.
const nfcPageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{{.Title}} - FilaBridge</title>
<style>
:root { color-scheme: dark; }
body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #09090b; color: #fafafa; margin: 0; padding: 20px; min-height: 100vh; display: flex; align-items: center; justify-content: center; }
.card { background: #18181b; border: 1px solid #27272a; border-radius: 16px; padding: 40px; text-align: center; max-width: 480px; width: 100%; box-shadow: 0 20px 60px rgba(0,0,0,.5); }
.icon { font-size: 56px; margin-bottom: 16px; }
h1 { font-size: 24px; margin: 0 0 12px; }
.msg { color: #a1a1aa; font-size: 16px; line-height: 1.5; margin-bottom: 24px; }
.details { background: #09090b; border: 1px solid #27272a; border-radius: 10px; padding: 16px 20px; margin-bottom: 24px; text-align: left; }
.row { display: flex; justify-content: space-between; padding: 8px 0; border-bottom: 1px solid #27272a; font-size: 14px; }
.row:last-child { border-bottom: none; }
.label { color: #a1a1aa; font-weight: 600; }
.steps { text-align: left; margin-bottom: 24px; }
.step { display: flex; align-items: center; gap: 12px; padding: 10px 0; font-size: 15px; }
.btn { display: inline-block; background: #fafafa; color: #09090b; text-decoration: none; font-weight: 600; padding: 12px 28px; border-radius: 10px; font-size: 15px; }
</style>
</head>
<body>
<div class="card">
<div class="icon">{{.Icon}}</div>
<h1>{{.Title}}</h1>
{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}
{{if .Steps}}<div class="steps">{{range .Steps}}<div class="step"><span>{{.Icon}}</span><span>{{.Text}}</span></div>{{end}}</div>{{end}}
{{if .Details}}<div class="details">{{range .Details}}<div class="row"><span class="label">{{.Label}}</span><span>{{.Value}}</span></div>{{end}}</div>{{end}}
<a href="/" class="btn">Back to Dashboard</a>
</div>
</body>
</html>`

var nfcPageTmpl = template.Must(template.New("nfc").Parse(nfcPageTemplate))

type nfcPageStep struct {
	Icon string
	Text string
}

type nfcPageDetail struct {
	Label string
	Value string
}

type nfcPageData struct {
	Title   string
	Icon    string
	Message string
	Steps   []nfcPageStep
	Details []nfcPageDetail
}

func renderNFCPage(c *gin.Context, status int, data nfcPageData) {
	c.Status(status)
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := nfcPageTmpl.Execute(c.Writer, data); err != nil {
		log.Printf("Error rendering NFC page: %v", err)
	}
}

func renderNFCError(c *gin.Context, status int, message string) {
	renderNFCPage(c, status, nfcPageData{
		Title:   "NFC Error",
		Icon:    "⚠️",
		Message: message,
	})
}

// nfcAssignHandler handles NFC tag scans.
func (ws *WebServer) nfcAssignHandler(c *gin.Context) {
	spoolIDStr := c.Query("spool")
	locationStr := c.Query("location")
	clientIP := nfc.ClientIP(c.ClientIP())

	// Generate session ID based on client IP
	sessionID := nfc.SessionIDForIP(clientIP)

	var spoolID int
	var printerName string
	var toolheadID int
	var err error

	if spoolIDStr != "" {
		spoolID, err = strconv.Atoi(spoolIDStr)
		if err != nil {
			renderNFCError(c, http.StatusBadRequest, "Invalid spool ID")
			return
		}
	}

	var locationName string
	var isPrinterLocation bool

	if locationStr != "" {
		if bambu.IsBambuLocation(locationStr) {
			tray, parseErr := bambu.ParseLocation(ws.bridge, locationStr)
			if parseErr != nil {
				renderNFCError(c, http.StatusBadRequest, parseErr.Error())
				return
			}
			if tray == nil {
				renderNFCError(c, http.StatusBadRequest, "Bambu tray not found for location")
				return
			}
			locationName = locationStr
			isPrinterLocation = true
		} else {
			printerName, toolheadID, locationName, isPrinterLocation, err = nfc.ParseLocationParam(ws.bridge, locationStr)
			if err != nil {
				renderNFCError(c, http.StatusBadRequest, err.Error())
				return
			}
		}
	}

	session, err := nfc.CreateOrUpdateSession(ws.bridge, sessionID, spoolID, printerName, toolheadID, locationName, isPrinterLocation)
	if err != nil {
		renderNFCError(c, http.StatusInternalServerError, "Failed to create session: "+err.Error())
		return
	}

	if session.IsComplete() {
		var err error
		if bambu.IsBambuLocation(session.LocationName) {
			tray, trayErr := bambu.ParseLocation(ws.bridge, session.LocationName)
			if trayErr != nil || tray == nil {
				renderNFCError(c, http.StatusInternalServerError, "Failed to resolve Bambu tray location")
				return
			}
			err = bambu.AssignSpoolToTray(ws.bridge, session.SpoolID, tray.UniqueID, session.LocationName)
		} else {
			err = ws.bridge.AssignSpoolToLocation(session.SpoolID, session.PrinterName, session.ToolheadID, session.LocationName, session.IsPrinterLocation)
		}
		if err != nil {
			renderNFCError(c, http.StatusInternalServerError, "Assignment failed: "+err.Error())
			return
		}

		ws.BroadcastStatus()

		nfc.DeleteSession(ws.bridge, sessionID)

		toolheadDisplayName := core.DefaultToolheadDisplayName(session.ToolheadID)
		printerConfigs, err := ws.bridge.GetAllPrinterConfigs()
		if err == nil {
			for printerID, cfg := range printerConfigs {
				if cfg.Name == session.PrinterName {
					if name, nameErr := ws.bridge.GetToolheadName(printerID, session.ToolheadID); nameErr == nil {
						toolheadDisplayName = name
					}
					break
				}
			}
		}

		details := []nfcPageDetail{
			{Label: "Spool ID", Value: strconv.Itoa(session.SpoolID)},
		}
		if session.IsPrinterLocation && session.PrinterName != "" {
			details = append(details,
				nfcPageDetail{Label: "Printer", Value: session.PrinterName},
				nfcPageDetail{Label: "Toolhead", Value: toolheadDisplayName},
			)
		} else {
			details = append(details, nfcPageDetail{Label: "Location", Value: session.LocationName})
		}

		renderNFCPage(c, http.StatusOK, nfcPageData{
			Title:   "Assignment Complete!",
			Icon:    "✅",
			Message: "Spool has been successfully assigned.",
			Details: details,
		})
		return
	}

	// Session not complete, show progress
	var message string
	if session.HasSpool && !session.HasLocation {
		message = fmt.Sprintf("Spool %d selected. Now scan a location tag.", session.SpoolID)
	} else if session.HasLocation && !session.HasSpool {
		message = fmt.Sprintf("Location '%s' selected. Now scan a spool tag.", session.LocationName)
	} else {
		message = "Session started. Scan a spool or location tag."
	}

	spoolStep := nfcPageStep{Icon: "⭕", Text: "Scan spool tag"}
	if session.HasSpool {
		spoolStep.Icon = "✅"
	} else if !session.HasLocation {
		spoolStep.Icon = "⏳"
	}
	locationStep := nfcPageStep{Icon: "⭕", Text: "Scan location tag"}
	if session.HasLocation {
		locationStep.Icon = "✅"
	} else if session.HasSpool {
		locationStep.Icon = "⏳"
	}

	renderNFCPage(c, http.StatusOK, nfcPageData{
		Title:   "NFC Scan Progress",
		Icon:    "📱",
		Message: message,
		Steps:   []nfcPageStep{spoolStep, locationStep},
	})
}

// normalizeNFCLocationName lowercases and removes spaces for dedup comparisons.
func normalizeNFCLocationName(name string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), " ", "")
}

func buildSpoolNFCURL(host string, spool spoolman.Spool) gin.H {
	url := fmt.Sprintf("http://%s/api/nfc/assign?spool=%d", host, spool.ID)

	colorHex := ""
	if spool.Filament != nil && spool.Filament.ColorHex != "" {
		colorHex = spool.Filament.ColorHex
		if !strings.HasPrefix(colorHex, "#") {
			colorHex = "#" + colorHex
		}
	}

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

// nfcUrlsHandler returns all available NFC URLs with QR codes.
func (ws *WebServer) nfcUrlsHandler(c *gin.Context) {
	var urls []gin.H

	host := requestHost(c)

	spools, err := ws.bridge.Spoolman.GetAllSpools()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	for _, spool := range spools {
		urls = append(urls, buildSpoolNFCURL(host, spool))
	}

	// Bambu AMS slot NFC URLs (canonical entries for printer slots)
	bambuLocationNames := make(map[string]struct{})
	if bambuURLs, err := bambu.GenerateNFCURLs(ws.bridge, host); err == nil {
		for _, entry := range bambuURLs {
			urls = append(urls, entry)
			if displayName, ok := entry["display_name"].(string); ok {
				bambuLocationNames[normalizeNFCLocationName(displayName)] = struct{}{}
			}
		}
	}

	spoolmanLocations, err := ws.bridge.Spoolman.GetLocations()
	if err != nil {
		log.Printf("Warning: Failed to get Spoolman locations: %v", err)
		spoolmanLocations = []spoolman.Location{}
	}

	// Spoolman storage locations only — skip slots already covered by Bambu entries
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

		locationParam := location.Name
		nfcURL := fmt.Sprintf("http://%s/api/nfc/assign?location=%s", host, neturl.QueryEscape(locationParam))

		qrCode, err := qrcode.Encode(nfcURL, qrcode.Medium, 256)
		if err != nil {
			log.Printf("Error generating QR code for Spoolman location %s: %v", locationParam, err)
			urls = append(urls, gin.H{
				"type":           "location",
				"location_type":  "storage",
				"location_name":  location.Name,
				"display_name":   location.Name,
				"url":            nfcURL,
				"qr_code_base64": "",
				"is_local_only":  false,
			})
			continue
		}

		qrCodeBase64 := base64.StdEncoding.EncodeToString(qrCode)
		urls = append(urls, gin.H{
			"type":           "location",
			"location_type":  "storage",
			"location_name":  location.Name,
			"display_name":   location.Name,
			"url":            nfcURL,
			"qr_code_base64": qrCodeBase64,
			"is_local_only":  false,
		})
	}

	// Sort URLs: spools first, then locations alphabetically by display name
	sort.Slice(urls, func(i, j int) bool {
		typeI := urls[i]["type"].(string)
		typeJ := urls[j]["type"].(string)

		if typeI != typeJ {
			return typeI == "spool"
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

// nfcSessionStatusHandler returns the current session status.
func (ws *WebServer) nfcSessionStatusHandler(c *gin.Context) {
	clientIP := nfc.ClientIP(c.ClientIP())
	sessionID := nfc.SessionIDForIP(clientIP)

	session, err := nfc.GetSession(ws.bridge, sessionID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"active": false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"active":              true,
		"session_id":          session.SessionID,
		"has_spool":           session.HasSpool,
		"has_location":        session.HasLocation,
		"spool_id":            session.SpoolID,
		"printer_name":        session.PrinterName,
		"toolhead_id":         session.ToolheadID,
		"location_name":       session.LocationName,
		"is_printer_location": session.IsPrinterLocation,
		"expires_at":          session.ExpiresAt,
	})
}
