package main

import (
	"fmt"
	"log"
	"strings"
)

// ProcessBambuWebhook handles tray_change and spool_usage events from Home Assistant.
func (b *FilamentBridge) ProcessBambuWebhook(payload BambuWebhookPayload, ha *HAClient) BambuWebhookResult {
	if b.spoolman == nil || b.config.SpoolmanURL == "" {
		return BambuWebhookResult{Status: "ignored", Reason: "spoolman not configured"}
	}

	idMap := map[string]string{}
	if ha != nil {
		if m, err := ha.GetEntityIdToUniqueIdMap(); err == nil {
			idMap = m
		}
	}

	switch payload.Event {
	case "spool_usage":
		result := b.processSpoolUsage(payload, ha, idMap)
		logBambuWebhookResult("spool_usage", payload.ActiveTrayID, result)
		return result
	case "tray_change":
		result := b.processTrayChange(payload, ha, idMap)
		logBambuWebhookResult("tray_change", payload.TrayEntityID, result)
		return result
	default:
		return BambuWebhookResult{Status: "ignored", Reason: "unknown event"}
	}
}

func logBambuWebhookResult(event, trayID string, result BambuWebhookResult) {
	switch result.Status {
	case "success":
		log.Printf("Webhook %s (%s): success spool=#%d action=%s deducted=%.2fg", event, trayID, result.SpoolID, result.Action, result.Deducted)
	case "no_match":
		log.Printf("Webhook %s (%s): no spool assigned — assign bobina no FilaBridge primeiro", event, trayID)
	case "ignored":
		log.Printf("Webhook %s (%s): ignored — %s", event, trayID, result.Reason)
	default:
		log.Printf("Webhook %s (%s): %s — %s", event, trayID, result.Status, result.Message)
	}
}

func (b *FilamentBridge) processSpoolUsage(payload BambuWebhookPayload, ha *HAClient, idMap map[string]string) BambuWebhookResult {
	weight := payload.UsedWeight
	lengthConverted := false
	if weight <= 0 && payload.UsedLength > 0 {
		weight = LengthToWeight(payload.UsedLength, payload.Material)
		lengthConverted = true
	}
	if weight <= 0 {
		return BambuWebhookResult{Status: "ignored", Reason: "no weight to deduct"}
	}
	if payload.ActiveTrayID == "" {
		return BambuWebhookResult{Status: "ignored", Reason: "no active_tray_id provided"}
	}

	trayUniqueID := payload.ActiveTrayID
	if ha != nil {
		trayUniqueID = ha.ResolveToUniqueID(payload.ActiveTrayID, idMap)
	}

	spool, err := b.spoolman.FindSpoolByActiveTray(payload.ActiveTrayID, trayUniqueID)
	if err != nil {
		return BambuWebhookResult{Status: "error", Message: err.Error()}
	}
	if spool == nil {
		return BambuWebhookResult{
			Status:  "no_match",
			Message: fmt.Sprintf("No spool assigned to tray %s. Assign a spool in FilaBridge first.", payload.ActiveTrayID),
		}
	}

	if lengthConverted && spool.Filament != nil && spool.Filament.Material != "" {
		weight = LengthToWeight(payload.UsedLength, spool.Filament.Material)
	}

	if err := b.spoolman.UseSpoolWeight(spool.ID, weight); err != nil {
		return BambuWebhookResult{Status: "error", Message: err.Error()}
	}

	tagStored := false
	if IsValidTrayUUID(payload.TrayUUID) {
		existing := GetSpoolExtraString(spool, spoolExtraFieldTag)
		if existing != payload.TrayUUID {
			if err := b.spoolman.SetSpoolTag(spool.ID, payload.TrayUUID); err != nil {
				log.Printf("Warning: failed to store RFID tag on spool #%d: %v", spool.ID, err)
			} else {
				tagStored = true
				log.Printf("Stored spool serial %q on spool #%d", payload.TrayUUID, spool.ID)
			}
		}
	}

	return BambuWebhookResult{
		Status:    "success",
		SpoolID:   spool.ID,
		Deducted:  weight,
		TagStored: tagStored,
	}
}

func (b *FilamentBridge) processTrayChange(payload BambuWebhookPayload, ha *HAClient, idMap map[string]string) BambuWebhookResult {
	if payload.TrayEntityID == "" {
		return BambuWebhookResult{Status: "ignored", Reason: "no tray_entity_id"}
	}

	trayUniqueID := payload.TrayEntityID
	if ha != nil {
		trayUniqueID = ha.ResolveToUniqueID(payload.TrayEntityID, idMap)
	}

	name := strings.TrimSpace(payload.Name)
	trayEmpty := name == "" || strings.EqualFold(name, "empty") || name == "unavailable"

	if trayEmpty {
		spool, err := b.spoolman.FindSpoolByActiveTray(payload.TrayEntityID, trayUniqueID)
		if err != nil {
			return BambuWebhookResult{Status: "error", Message: err.Error()}
		}
		if spool == nil {
			return BambuWebhookResult{Status: "ignored", Reason: "tray empty and no spool was assigned"}
		}
		if err := b.UnassignBambuTray(trayUniqueID); err != nil {
			return BambuWebhookResult{Status: "error", Message: err.Error()}
		}
		return BambuWebhookResult{Status: "success", Action: "unassigned", SpoolID: spool.ID, Reason: "tray_empty"}
	}

	if !IsValidTrayUUID(payload.TrayUUID) {
		return BambuWebhookResult{Status: "ignored", Reason: "no valid RFID tag for auto-assign"}
	}

	spool, err := b.spoolman.FindSpoolByTag(payload.TrayUUID)
	if err != nil {
		return BambuWebhookResult{Status: "error", Message: err.Error()}
	}
	if spool == nil {
		return BambuWebhookResult{Status: "ignored", Reason: "no spool found for RFID tag"}
	}

	if err := b.spoolman.AssignSpoolToTray(spool.ID, trayUniqueID); err != nil {
		return BambuWebhookResult{Status: "error", Message: err.Error()}
	}

	return BambuWebhookResult{Status: "success", Action: "assigned", SpoolID: spool.ID}
}
