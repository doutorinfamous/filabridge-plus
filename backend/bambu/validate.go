package bambu

import (
	"fmt"

	"filabridge/homeassistant"
)

// HAEntityCheck is one required FilaBridge entity in Home Assistant.
type HAEntityCheck struct {
	EntityID string `json:"entity_id"`
	Found    bool   `json:"found"`
	State    string `json:"state,omitempty"`
	Required bool   `json:"required"`
	Hint     string `json:"hint,omitempty"`
}

// HAValidation summarizes HA package entity health for one printer.
type HAValidation struct {
	Prefix       string          `json:"prefix"`
	PackageFile  string          `json:"package_file"`
	AllOK        bool            `json:"all_ok"`
	MeterMissing bool            `json:"meter_missing"`
	Checks       []HAEntityCheck `json:"checks"`
	FixSteps     []string        `json:"fix_steps,omitempty"`
}

func haEntityChecks(prefix string) []HAEntityCheck {
	p := NormalizePrefix(prefix)
	return []HAEntityCheck{
		{
			EntityID: fmt.Sprintf("sensor.filabridge_%s_filament_usage", p),
			Required: true,
			Hint:     "Template sensor: grams used during the print",
		},
		{
			EntityID: fmt.Sprintf("sensor.filabridge_%s_filament_usage_meter", p),
			Required: true,
			Hint:     "Utility meter: required for utility_meter.calibrate and Spoolman debit",
		},
		{
			EntityID: fmt.Sprintf("input_number.filabridge_%s_last_tray", p),
			Required: true,
			Hint:     "Helper: last active tray when print starts",
		},
		{
			EntityID: fmt.Sprintf("sensor.filabridge_%s_active_tray", p),
			Required: true,
			Hint:     "Template sensor: active AMS tray",
		},
	}
}

// ValidateHAEntities checks that FilaBridge package entities exist in HA.
func ValidateHAEntities(prefix string, states []homeassistant.State) HAValidation {
	stateByID := make(map[string]string, len(states))
	for _, s := range states {
		stateByID[s.EntityID] = s.State
	}

	checks := haEntityChecks(prefix)
	allOK := true
	meterMissing := false
	for i := range checks {
		state, ok := stateByID[checks[i].EntityID]
		checks[i].Found = ok
		if ok {
			checks[i].State = state
		}
		if checks[i].Required && !ok {
			allOK = false
			if checks[i].EntityID == fmt.Sprintf("sensor.filabridge_%s_filament_usage_meter", NormalizePrefix(prefix)) {
				meterMissing = true
			}
		}
	}

	result := HAValidation{
		Prefix:       NormalizePrefix(prefix),
		PackageFile:  "filabridge_" + NormalizePrefix(prefix) + ".yaml",
		AllOK:        allOK,
		MeterMissing: meterMissing,
		Checks:       checks,
	}
	if !allOK {
		result.FixSteps = []string{
			"In FilaBridge+, click HA Config and download the full YAML (must contain utility_meter:, template:, automation:).",
			"Replace config/packages/" + result.PackageFile + " in Home Assistant (lowercase filename).",
			"Fully restart Home Assistant (do not only reload automations).",
			"Confirm all 4 entities in Developer Tools → States.",
		}
		if meterMissing {
			result.FixSteps = append([]string{
				"utility_meter.calibrate is unknown when sensor.filabridge_*_filament_usage_meter is missing — do not remove that action from the automation.",
			}, result.FixSteps...)
		}
	}
	return result
}
