package core

// Printer states
const (
	StateIdle          = "IDLE"
	StatePrinting      = "PRINTING"
	StateFinished      = "FINISHED"
	StateError         = "error"
	StateOffline       = "offline"
	StateNotConfigured = "not_configured"
)

// Default configuration values
const (
	DefaultSpoolmanURL          = "http://localhost:7912"
	DefaultWebPort              = "5001"
	DefaultPublicWebPort        = "5000"
	DefaultPollInterval         = 30
	DefaultLocationSyncInterval = 5 // minutes
	DefaultDBFileName           = "filabridge.db"
)

// Database configuration keys
const (
	ConfigKeyPrinterIPs                 = "printer_ips"
	ConfigKeyAPIKey                     = "printer_api_key"
	ConfigKeySpoolmanURL                = "spoolman_url"
	ConfigKeyPollInterval               = "poll_interval"
	ConfigKeyLocationSyncInterval       = "location_sync_interval"
	ConfigKeyWebPort                    = "web_port"
	ConfigKeyPrinterTimeout             = "printer_timeout"
	ConfigKeyPrinterFileDownloadTimeout = "printer_file_download_timeout"
	// Legacy config keys kept for migration from PrusaLink installs.
	ConfigKeyLegacyAPIKey                     = "prusalink_api_key"
	ConfigKeyLegacyPrinterTimeout             = "prusalink_timeout"
	ConfigKeyLegacyPrinterFileDownloadTimeout = "prusalink_file_download_timeout"
	ConfigKeySpoolmanTimeout                  = "spoolman_timeout"
	ConfigKeySpoolmanUsername                 = "spoolman_username"
	ConfigKeySpoolmanPassword                 = "spoolman_password"
	ConfigKeyAutoAssignPreviousSpoolEnabled   = "auto_assign_previous_spool_enabled"
	ConfigKeyAutoAssignPreviousSpoolLocation  = "auto_assign_previous_spool_location"
	ConfigKeySyncFilamentToPrinter            = "sync_filament_to_printer"
	ConfigKeyHAURL                            = "ha_url"
	ConfigKeyHAToken                          = "ha_token"
	ConfigKeyFilabridgePublicURL              = "filabridge_public_url"
)

// Printer drivers
const (
	DriverMoonraker = "moonraker"
	DriverBambuHA   = "bambu_ha"
)

// Printer slot types (printer_slots.slot_type). A slot is any assignable
// filament position on a printer: a Moonraker toolhead, a Bambu AMS tray or
// an external spool holder. Future printer types add new slot types here.
const (
	SlotTypeToolhead = "toolhead"
	SlotTypeAMSTray  = "ams_tray"
	SlotTypeExternal = "external"
)

// ConfigKeySlotsTrayBackfillDone tracks whether tray spool assignments were
// backfilled from Spoolman (extra.active_tray) into printer_slots after the
// v3 schema migration.
const ConfigKeySlotsTrayBackfillDone = "slots_tray_backfill_done"

// HTTP timeouts
const (
	PrinterTimeout             = 10  // seconds
	PrinterFileDownloadTimeout = 300 // seconds for file downloads
	SpoolmanTimeout            = 10  // seconds
)
