package bambu

import (
	"fmt"
	"log"
	"strings"

	"filabridge/core"
	"filabridge/homeassistant"
	"filabridge/spoolman"
)

// ProcessWebhook handles tray_change and spool_usage events from Home Assistant.
func ProcessWebhook(b *core.FilamentBridge, payload WebhookPayload, ha *homeassistant.Client) WebhookResult {
	if b.Spoolman == nil || b.Config.SpoolmanURL == "" {
		return WebhookResult{Status: "ignored", Reason: "spoolman not configured"}
	}

	idMap := map[string]string{}
	if ha != nil {
		if m, err := ha.GetEntityIdToUniqueIdMap(); err == nil {
			idMap = m
		}
	}

	switch payload.Event {
	case "spool_usage":
		result := processSpoolUsage(b, payload, ha, idMap)
		logWebhookResult("spool_usage", payload.ActiveTrayID, result)
		return result
	case "tray_change":
		result := processTrayChange(b, payload, ha, idMap)
		logWebhookResult("tray_change", payload.TrayEntityID, result)
		return result
	case "print_started":
		result := processPrintStarted(b, payload)
		logWebhookResult("print_started", payload.PrinterPrefix, result)
		return result
	case "print_finished":
		result := processPrintFinished(b, payload)
		logWebhookResult("print_finished", payload.PrinterPrefix, result)
		return result
	default:
		return WebhookResult{Status: "ignored", Reason: "unknown event"}
	}
}

// processPrintStarted opens a print job for a Bambu printer.
func processPrintStarted(b *core.FilamentBridge, payload WebhookPayload) WebhookResult {
	printerID, err := FindPrinterIDByPrefix(b, payload.PrinterPrefix)
	if err != nil {
		return WebhookResult{Status: "error", Message: err.Error()}
	}
	if printerID == "" {
		return WebhookResult{Status: "ignored", Reason: "unknown printer_prefix"}
	}

	jobName := strings.TrimSpace(payload.JobName)
	if jobName == "" {
		jobName = "unknown"
	}
	if _, err := b.StartPrintJob(printerID, jobName); err != nil {
		return WebhookResult{Status: "error", Message: err.Error()}
	}
	return WebhookResult{Status: "success", Action: "job_started"}
}

// processPrintFinished closes the open print job for a Bambu printer.
func processPrintFinished(b *core.FilamentBridge, payload WebhookPayload) WebhookResult {
	printerID, err := FindPrinterIDByPrefix(b, payload.PrinterPrefix)
	if err != nil {
		return WebhookResult{Status: "error", Message: err.Error()}
	}
	if printerID == "" {
		return WebhookResult{Status: "ignored", Reason: "unknown printer_prefix"}
	}

	// ha-bambulab reports "finish" on success; anything else closing a print
	// (idle after failure, etc.) counts as failed.
	status := core.JobStatusFailed
	if strings.EqualFold(strings.TrimSpace(payload.PrintState), "finish") {
		status = core.JobStatusCompleted
	}

	jobName := strings.TrimSpace(payload.JobName)
	if jobName != "" {
		if err := b.FinishPrintJob(printerID, jobName, status); err != nil {
			return WebhookResult{Status: "error", Message: err.Error()}
		}
	} else if err := b.FinishLatestOpenJob(printerID, status); err != nil {
		return WebhookResult{Status: "error", Message: err.Error()}
	}
	return WebhookResult{Status: "success", Action: "job_finished"}
}

// recordTrayUsage logs a filament usage event for the printer's open job.
// History recording failures never fail the webhook (the Spoolman debit already happened).
func recordTrayUsage(b *core.FilamentBridge, payload WebhookPayload, trayUniqueID string, spoolID int, grams float64) {
	printerID, err := FindTrayPrinterID(b, trayUniqueID)
	if err != nil || printerID == "" {
		if prefixID, prefixErr := FindPrinterIDByPrefix(b, payload.PrinterPrefix); prefixErr == nil && prefixID != "" {
			printerID = prefixID
		}
	}
	if printerID == "" {
		log.Printf("Warning: could not resolve printer for tray %s — usage not recorded in history", trayUniqueID)
		return
	}

	jobID, err := b.GetOrCreateOpenJob(printerID, strings.TrimSpace(payload.JobName))
	if err != nil {
		log.Printf("Warning: failed to resolve print job for %s: %v", printerID, err)
		return
	}
	if err := b.LogTrayUsage(jobID, printerID, trayUniqueID, spoolID, grams); err != nil {
		log.Printf("Warning: failed to record filament usage for %s tray %s: %v", printerID, trayUniqueID, err)
	}
}

func logWebhookResult(event, trayID string, result WebhookResult) {
	switch result.Status {
	case "success":
		log.Printf("Webhook %s (%s): success spool=#%d action=%s deducted=%.2fg", event, trayID, result.SpoolID, result.Action, result.Deducted)
	case "no_match":
		log.Printf("Webhook %s (%s): no spool assigned — assign spool in FilaBridge first", event, trayID)
	case "ignored":
		log.Printf("Webhook %s (%s): ignored — %s", event, trayID, result.Reason)
	default:
		log.Printf("Webhook %s (%s): %s — %s", event, trayID, result.Status, result.Message)
	}
}

// findAssignedSpoolForTray resolves the spool assigned to a tray. printer_slots
// is the source of truth; the legacy Spoolman extra.active_tray lookup is kept
// as a fallback (self-healing the local slot when it hits).
func findAssignedSpoolForTray(b *core.FilamentBridge, trayRef, trayUniqueID string) (*spoolman.Spool, error) {
	spoolID, err := FindSpoolIDForTrayLocal(b, trayUniqueID)
	if err != nil {
		return nil, err
	}
	if spoolID > 0 {
		return b.Spoolman.GetSpool(spoolID)
	}

	spool, err := b.Spoolman.FindSpoolByActiveTray(trayRef, trayUniqueID)
	if err != nil || spool == nil {
		return nil, err
	}
	printerID, _ := FindTrayPrinterID(b, trayUniqueID)
	if err := b.SetSlotSpool(trayUniqueID, printerID, core.SlotTypeAMSTray, "", spool.ID); err != nil {
		log.Printf("Warning: failed to self-heal local slot for tray %s: %v", trayUniqueID, err)
	}
	return spool, nil
}

// resolveTrayUniqueID maps a HA entity_id (or unique_id) to the tray unique_id stored in Spoolman.
func resolveTrayUniqueID(b *core.FilamentBridge, trayRef string, ha *homeassistant.Client, idMap map[string]string) string {
	if trayRef == "" {
		return ""
	}
	if ha != nil {
		if uid := ha.ResolveToUniqueID(trayRef, idMap); uid != trayRef {
			return uid
		}
	}
	if tray, err := FindTrayByEntityID(b, trayRef); err == nil && tray != nil {
		return tray.UniqueID
	}
	candidate := trayRef
	if strings.HasPrefix(candidate, "sensor.") {
		candidate = strings.TrimPrefix(candidate, "sensor.")
	}
	if tray, err := FindTrayByUniqueID(b, candidate); err == nil && tray != nil {
		return tray.UniqueID
	}
	return candidate
}

func processSpoolUsage(b *core.FilamentBridge, payload WebhookPayload, ha *homeassistant.Client, idMap map[string]string) WebhookResult {
	weight := payload.UsedWeight
	lengthConverted := false
	if weight <= 0 && payload.UsedLength > 0 {
		weight = spoolman.LengthToWeight(payload.UsedLength, payload.Material)
		lengthConverted = true
	}
	if weight <= 0 {
		return WebhookResult{Status: "ignored", Reason: "no weight to deduct"}
	}
	if payload.ActiveTrayID == "" {
		return WebhookResult{Status: "ignored", Reason: "no active_tray_id provided"}
	}

	trayUniqueID := resolveTrayUniqueID(b, payload.ActiveTrayID, ha, idMap)

	spool, err := findAssignedSpoolForTray(b, payload.ActiveTrayID, trayUniqueID)
	if err != nil {
		return WebhookResult{Status: "error", Message: err.Error()}
	}
	if spool == nil {
		return WebhookResult{
			Status:  "no_match",
			Message: fmt.Sprintf("No spool assigned to tray %s. Assign a spool in FilaBridge first.", payload.ActiveTrayID),
		}
	}

	if lengthConverted && spool.Filament != nil && spool.Filament.Material != "" {
		weight = spoolman.LengthToWeight(payload.UsedLength, spool.Filament.Material)
	}

	if err := b.Spoolman.UseSpoolWeight(spool.ID, weight); err != nil {
		return WebhookResult{Status: "error", Message: err.Error()}
	}

	recordTrayUsage(b, payload, trayUniqueID, spool.ID, weight)

	tagStored := false
	if spoolman.IsValidTrayUUID(payload.TrayUUID) {
		existing := spoolman.GetSpoolExtraString(spool, spoolman.ExtraFieldTag)
		if existing != payload.TrayUUID {
			if err := b.Spoolman.SetSpoolTag(spool.ID, payload.TrayUUID); err != nil {
				log.Printf("Warning: failed to store RFID tag on spool #%d: %v", spool.ID, err)
			} else {
				tagStored = true
				log.Printf("Stored spool serial %q on spool #%d", payload.TrayUUID, spool.ID)
			}
		}
	}

	return WebhookResult{
		Status:    "success",
		SpoolID:   spool.ID,
		Deducted:  weight,
		TagStored: tagStored,
	}
}

func processTrayChange(b *core.FilamentBridge, payload WebhookPayload, ha *homeassistant.Client, idMap map[string]string) WebhookResult {
	if payload.TrayEntityID == "" {
		return WebhookResult{Status: "ignored", Reason: "no tray_entity_id"}
	}

	trayUniqueID := resolveTrayUniqueID(b, payload.TrayEntityID, ha, idMap)

	name := strings.TrimSpace(payload.Name)
	trayEmpty := name == "" || strings.EqualFold(name, "empty") || name == "unavailable"

	if trayEmpty {
		spool, err := findAssignedSpoolForTray(b, payload.TrayEntityID, trayUniqueID)
		if err != nil {
			return WebhookResult{Status: "error", Message: err.Error()}
		}
		if spool == nil {
			return WebhookResult{Status: "ignored", Reason: "tray empty and no spool was assigned"}
		}
		if err := UnassignTray(b, trayUniqueID); err != nil {
			return WebhookResult{Status: "error", Message: err.Error()}
		}
		return WebhookResult{Status: "success", Action: "unassigned", SpoolID: spool.ID, Reason: "tray_empty"}
	}

	if !spoolman.IsValidTrayUUID(payload.TrayUUID) {
		return WebhookResult{Status: "ignored", Reason: "no valid RFID tag for auto-assign"}
	}

	spool, err := b.Spoolman.FindSpoolByTag(payload.TrayUUID)
	if err != nil {
		return WebhookResult{Status: "error", Message: err.Error()}
	}
	if spool == nil {
		return WebhookResult{Status: "ignored", Reason: "no spool found for RFID tag"}
	}

	displayName := ""
	if tray, err := FindTrayByUniqueID(b, trayUniqueID); err == nil && tray != nil {
		displayName = tray.DisplayName
	}
	if err := AssignSpoolToTray(b, spool.ID, trayUniqueID, displayName); err != nil {
		return WebhookResult{Status: "error", Message: err.Error()}
	}

	return WebhookResult{Status: "success", Action: "assigned", SpoolID: spool.ID}
}
