package core

import "time"

// ToolheadMapping represents a mapping between a printer toolhead and a spool.
type ToolheadMapping struct {
	PrinterID   string    `json:"printer_id"`
	PrinterName string    `json:"printer_name"`
	ToolheadID  int       `json:"toolhead_id"`
	SpoolID     int       `json:"spool_id"`
	MappedAt    time.Time `json:"mapped_at"`
	DisplayName string    `json:"display_name,omitempty"` // Custom toolhead name or empty for default
}

// Print error kinds.
const (
	PrintErrorKindProcessing     = "processing_error"
	PrintErrorKindUsageConfirm   = "usage_confirmation"
)

// PrintErrorInput carries structured data when recording a print processing failure.
type PrintErrorInput struct {
	PrinterID   string
	PrinterName string
	JobName     string
	Error       string
	ToolheadID  int     // -1 when unknown
	Grams       float64 // 0 when unknown
	Kind        string  // empty defaults to processing_error
	SpoolID     int     // >0 when spool is already known (usage confirmation)
	FinalStatus string  // job status to apply when all confirmations are resolved
}

// Print error resolution actions.
const (
	ResolveActionAssignSpool = "assign_spool"
	ResolveActionDebitSpool  = "debit_spool"
	ResolveActionDismiss     = "dismiss"
)

// PrintError represents a failed print processing attempt.
type PrintError struct {
	ID           string    `json:"id"`
	Kind         string    `json:"kind,omitempty"`
	PrinterID    string    `json:"printer_id,omitempty"`
	PrinterName  string    `json:"printer_name"`
	Filename     string    `json:"filename"`
	JobName      string    `json:"job_name,omitempty"`
	ToolheadID   *int      `json:"toolhead_id,omitempty"`
	SpoolID      *int      `json:"spool_id,omitempty"`
	Grams        float64   `json:"grams,omitempty"`
	Error        string    `json:"error"`
	FinalStatus  string    `json:"final_status,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
	Acknowledged bool      `json:"acknowledged"`
}

// PrinterStatus represents the current status of all printers.
type PrinterStatus struct {
	Printers         map[string]PrinterData             `json:"printers"`
	ToolheadMappings map[string]map[int]ToolheadMapping `json:"toolhead_mappings"`
	Timestamp        time.Time                          `json:"timestamp"`
}

// PrinterData represents data for a single printer.
type PrinterData struct {
	Name          string   `json:"name"`
	State         string   `json:"state"`
	JobName       string   `json:"job_name,omitempty"`
	Progress      float64  `json:"progress,omitempty"`
	PrintDuration float64  `json:"print_duration,omitempty"`
	TimeRemaining *float64 `json:"time_remaining,omitempty"`
	CurrentLayer  *int     `json:"current_layer,omitempty"`
	TotalLayer    *int     `json:"total_layer,omitempty"`
}
