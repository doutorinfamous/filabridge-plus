package snapmaker

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"unicode"

	"filabridge/core"
	"filabridge/spoolman"
)

const (
	defaultSnapmakerVendor   = "generic"
	defaultSnapmakerSubType  = "generic"
	defaultSnapmakerMaterial = "PLA"
	defaultFilamentColorRGBA = "FFFFFFFF"
)

var snapmakerGcodeTokenSanitizer = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// SnapmakerFilamentConfig holds print_task_config filament fields for one extruder.
type SnapmakerFilamentConfig struct {
	Vendor   string
	Type     string
	SubType  string
	ColorRGBA string
}

// SpoolToSnapmakerFilamentConfig maps a Spoolman spool to Snapmaker print_task_config fields.
func SpoolToSnapmakerFilamentConfig(spool spoolman.Spool) SnapmakerFilamentConfig {
	vendor := sanitizeSnapmakerToken(spool.Brand)
	if vendor == "" {
		vendor = defaultSnapmakerVendor
	}

	material := strings.ToUpper(strings.TrimSpace(spool.Material))
	if material == "" && spool.Filament != nil {
		material = strings.ToUpper(strings.TrimSpace(spool.Filament.Material))
	}
	if material == "" {
		material = defaultSnapmakerMaterial
	}

	subType := inferSnapmakerSubType(spool)

	return SnapmakerFilamentConfig{
		Vendor:    vendor,
		Type:      material,
		SubType:   subType,
		ColorRGBA: spoolColorToRGBA(spool),
	}
}

func inferSnapmakerSubType(spool spoolman.Spool) string {
	name := strings.ToLower(strings.TrimSpace(spool.Name))
	if spool.Filament != nil && spool.Filament.Name != "" {
		name = strings.ToLower(strings.TrimSpace(spool.Filament.Name))
	}

	switch {
	case strings.Contains(name, "wood"):
		return "Wood"
	case strings.Contains(name, "matte"):
		return "Matte"
	case strings.Contains(name, "silk"):
		return "Silk"
	default:
		return defaultSnapmakerSubType
	}
}

func spoolColorToRGBA(spool spoolman.Spool) string {
	colorHex := ""
	if spool.Filament != nil {
		colorHex = spool.Filament.ColorHex
	}
	return colorHexToRGBA(colorHex)
}

func colorHexToRGBA(colorHex string) string {
	colorHex = strings.TrimSpace(strings.TrimPrefix(colorHex, "#"))
	if len(colorHex) == 6 {
		return strings.ToUpper(colorHex) + "FF"
	}
	if len(colorHex) == 8 {
		return strings.ToUpper(colorHex)
	}
	return defaultFilamentColorRGBA
}

func sanitizeSnapmakerToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '-', r == '_':
			b.WriteRune(r)
		case unicode.IsSpace(r):
			b.WriteRune('_')
		}
	}

	return snapmakerGcodeTokenSanitizer.ReplaceAllString(b.String(), "")
}

// BuildSetPrintFilamentConfigGcode builds the Snapmaker SET_PRINT_FILAMENT_CONFIG command.
func BuildSetPrintFilamentConfigGcode(toolheadID int, cfg SnapmakerFilamentConfig) string {
	return fmt.Sprintf(
		"SET_PRINT_FILAMENT_CONFIG CONFIG_EXTRUDER=%d VENDOR=%s FILAMENT_TYPE=%s FILAMENT_SUBTYPE=%s FILAMENT_COLOR_RGBA=%s FORCE=1",
		toolheadID,
		cfg.Vendor,
		cfg.Type,
		cfg.SubType,
		cfg.ColorRGBA,
	)
}

// BuildClearFilamentConfigGcode resets an extruder slot to the default NONE filament profile.
func BuildClearFilamentConfigGcode(toolheadID int) string {
	return fmt.Sprintf(
		"SET_PRINT_FILAMENT_CONFIG CONFIG_EXTRUDER=%d VENDOR=NONE FILAMENT_TYPE=NONE FILAMENT_SUBTYPE=NONE FILAMENT_COLOR_RGBA=%s FORCE=1",
		toolheadID,
		defaultFilamentColorRGBA,
	)
}

// TrySyncFilamentToToolhead pushes Spoolman filament metadata to Snapmaker print_task_config.
// When spoolID is 0, the extruder filament profile is cleared on the printer.
func TrySyncFilamentToToolhead(bridge *core.FilamentBridge, printerID string, toolheadID int, spoolID int) error {
	if bridge == nil {
		return fmt.Errorf("filament bridge is nil")
	}

	enabled, err := bridge.GetSyncFilamentToPrinterEnabled()
	if err != nil {
		return fmt.Errorf("failed to read sync_filament_to_printer setting: %w", err)
	}
	if !enabled {
		return nil
	}

	if toolheadID < 0 || toolheadID >= MaxPhysicalExtruders {
		return fmt.Errorf("toolhead %d is out of Snapmaker range [0, %d)", toolheadID, MaxPhysicalExtruders)
	}

	snapshot := bridge.GetConfigSnapshot()
	if snapshot == nil {
		return fmt.Errorf("configuration snapshot unavailable")
	}

	printerConfig, ok := snapshot.Printers[printerID]
	if !ok {
		return fmt.Errorf("printer %q not found in configuration", printerID)
	}

	driver := printerConfig.Driver
	if driver == "" {
		driver = core.DriverMoonraker
	}
	if driver != core.DriverMoonraker {
		return nil
	}

	if strings.TrimSpace(printerConfig.IPAddress) == "" {
		return fmt.Errorf("printer %q has no Moonraker address configured", printerID)
	}

	timeout := snapshot.PrinterTimeout
	if timeout <= 0 {
		timeout = core.PrinterTimeout
	}

	client := NewMoonrakerClient(printerConfig.IPAddress, printerConfig.APIKey, timeout, snapshot.PrinterFileDownloadTimeout)

	var script string
	if spoolID <= 0 {
		script = BuildClearFilamentConfigGcode(toolheadID)
	} else {
		spool, err := bridge.Spoolman.GetSpool(spoolID)
		if err != nil {
			return fmt.Errorf("failed to load spool %d from Spoolman: %w", spoolID, err)
		}
		cfg := SpoolToSnapmakerFilamentConfig(*spool)
		script = BuildSetPrintFilamentConfigGcode(toolheadID, cfg)
	}

	if err := client.RunGcodeScript(script); err != nil {
		return fmt.Errorf("failed to run filament sync gcode on %q: %w", printerConfig.Name, err)
	}

	if spoolID <= 0 {
		log.Printf("Cleared Snapmaker filament config on %s toolhead %d", printerConfig.Name, toolheadID)
	} else {
		log.Printf("Synced Spoolman spool %d to Snapmaker %s toolhead %d via print_task_config", spoolID, printerConfig.Name, toolheadID)
	}

	return nil
}
