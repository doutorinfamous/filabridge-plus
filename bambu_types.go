package main

// BambuTray represents a single AMS tray or external spool from ha-bambulab.
type BambuTray struct {
	EntityID        string  `json:"entity_id"`
	UniqueID        string  `json:"unique_id"`
	TrayNumber      int     `json:"tray_number"`
	AMSNumber       int     `json:"ams_number"`
	IsExternal      bool    `json:"is_external"`
	Name            string  `json:"name,omitempty"`
	Color           string  `json:"color,omitempty"`
	Material        string  `json:"material,omitempty"`
	TrayUUID        string  `json:"tray_uuid,omitempty"`
	RemainingWeight float64 `json:"remaining_weight,omitempty"`
	DisplayName     string  `json:"display_name,omitempty"`
	AssignedSpoolID *int    `json:"assigned_spool_id,omitempty"`
}

// BambuAMS represents an AMS unit on a Bambu printer.
type BambuAMS struct {
	EntityID  string      `json:"entity_id"`
	Name      string      `json:"name"`
	AMSNumber int         `json:"ams_number"`
	Trays     []BambuTray `json:"trays"`
}

// BambuPrinter represents a discovered or registered Bambu Lab printer via HA.
type BambuPrinter struct {
	EntityID            string      `json:"entity_id"`
	DeviceID            string      `json:"device_id"`
	Prefix              string      `json:"prefix"`
	Name                string      `json:"name"`
	State               string      `json:"state,omitempty"`
	JobName             string      `json:"job_name,omitempty"`
	Progress            float64     `json:"progress,omitempty"`
	PrintDuration       float64     `json:"print_duration,omitempty"`
	TimeRemaining       *float64    `json:"time_remaining,omitempty"`
	CurrentLayer        *int        `json:"current_layer,omitempty"`
	TotalLayer          *int        `json:"total_layer,omitempty"`
	AMSUnits            []BambuAMS  `json:"ams_units"`
	ExternalSpools      []BambuTray `json:"external_spools"`
	CurrentStageEntity  string      `json:"current_stage_entity,omitempty"`
	PrintWeightEntity   string      `json:"print_weight_entity,omitempty"`
	PrintProgressEntity string      `json:"print_progress_entity,omitempty"`
	Registered          bool        `json:"registered,omitempty"`
	PrinterID           string      `json:"printer_id,omitempty"`
}

// BambuTrayInfo is used for HA config generation.
type BambuTrayInfo struct {
	EntityID    string
	AMSNumber   int
	TrayNumber  int
	CompositeID int
}

// BambuWebhookPayload is the inbound webhook body from Home Assistant.
type BambuWebhookPayload struct {
	Event          string  `json:"event"`
	ActiveTrayID   string  `json:"active_tray_id"`
	TrayEntityID   string  `json:"tray_entity_id"`
	TrayUUID       string  `json:"tray_uuid"`
	Name           string  `json:"name"`
	Material       string  `json:"material"`
	Color          string  `json:"color"`
	UsedWeight     float64 `json:"used_weight"`
	UsedLength     float64 `json:"used_length"`
}

// BambuWebhookResult is returned from webhook processing.
type BambuWebhookResult struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Action  string      `json:"action,omitempty"`
	SpoolID int         `json:"spool_id,omitempty"`
	Deducted float64    `json:"deducted,omitempty"`
	Reason  string      `json:"reason,omitempty"`
	TagStored bool      `json:"tag_stored,omitempty"`
}

// TrayAssignRequest is the body for POST /api/trays/assign.
type TrayAssignRequest struct {
	SpoolID      int    `json:"spool_id"`
	TrayUniqueID string `json:"tray_unique_id"`
}
