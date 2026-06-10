package main

import "fmt"

// HAEntityCheck is one required FilaBridge entity in Home Assistant.
type HAEntityCheck struct {
	EntityID string `json:"entity_id"`
	Found    bool   `json:"found"`
	State    string `json:"state,omitempty"`
	Required bool   `json:"required"`
	Hint     string `json:"hint,omitempty"`
}

// FilaBridgeHAValidation summarizes HA package entity health for one printer.
type FilaBridgeHAValidation struct {
	Prefix       string          `json:"prefix"`
	PackageFile  string          `json:"package_file"`
	AllOK        bool            `json:"all_ok"`
	MeterMissing bool            `json:"meter_missing"`
	Checks       []HAEntityCheck `json:"checks"`
	FixSteps     []string        `json:"fix_steps,omitempty"`
}

func filaBridgeHAEntityChecks(prefix string) []HAEntityCheck {
	p := normalizeBambuHAPrefix(prefix)
	return []HAEntityCheck{
		{
			EntityID: fmt.Sprintf("sensor.filabridge_%s_filament_usage", p),
			Required: true,
			Hint:     "Template sensor: gramas usadas durante o print",
		},
		{
			EntityID: fmt.Sprintf("sensor.filabridge_%s_filament_usage_meter", p),
			Required: true,
			Hint:     "Utility meter: necessário para utility_meter.calibrate e débito no Spoolman",
		},
		{
			EntityID: fmt.Sprintf("input_number.filabridge_%s_last_tray", p),
			Required: true,
			Hint:     "Helper: última bandeja ativa ao iniciar impressão",
		},
		{
			EntityID: fmt.Sprintf("sensor.filabridge_%s_active_tray", p),
			Required: true,
			Hint:     "Template sensor: bandeja AMS ativa",
		},
	}
}

// ValidateFilaBridgeHAEntities checks that FilaBridge package entities exist in HA.
func ValidateFilaBridgeHAEntities(prefix string, states []HAState) FilaBridgeHAValidation {
	stateByID := make(map[string]string, len(states))
	for _, s := range states {
		stateByID[s.EntityID] = s.State
	}

	checks := filaBridgeHAEntityChecks(prefix)
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
			if checks[i].EntityID == fmt.Sprintf("sensor.filabridge_%s_filament_usage_meter", normalizeBambuHAPrefix(prefix)) {
				meterMissing = true
			}
		}
	}

	result := FilaBridgeHAValidation{
		Prefix:       normalizeBambuHAPrefix(prefix),
		PackageFile:  "filabridge_" + normalizeBambuHAPrefix(prefix) + ".yaml",
		AllOK:        allOK,
		MeterMissing: meterMissing,
		Checks:       checks,
	}
	if !allOK {
		result.FixSteps = []string{
			"No FilaBridge, clique HA Config e baixe o YAML completo (deve conter utility_meter:, template:, automation:).",
			"Substitua config/packages/" + result.PackageFile + " no Home Assistant (nome em minúsculas).",
			"Reinicie o Home Assistant por completo (não só recarregar automações).",
			"Confirme as 4 entidades em Ferramentas de desenvolvedor → Estados.",
		}
		if meterMissing {
			result.FixSteps = append([]string{
				"utility_meter.calibrate fica desconhecida quando sensor.filabridge_*_filament_usage_meter não existe — não remova essa ação da automação.",
			}, result.FixSteps...)
		}
	}
	return result
}
