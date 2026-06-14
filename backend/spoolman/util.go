package spoolman

import (
	"encoding/json"
	"strings"
)

// materialDensityGPerCm3 maps filament material names to density (g/cm³).
var materialDensityGPerCm3 = map[string]float64{
	"PLA":   1.24,
	"PLA+":  1.24,
	"PETG":  1.27,
	"ABS":   1.04,
	"ASA":   1.07,
	"TPU":   1.21,
	"PC":    1.20,
	"PA":    1.14,
	"PA-CF": 1.35,
	"PA-GF": 1.36,
	"PVA":   1.23,
	"HIPS":  1.04,
}

// IsValidTrayUUID returns true if tray_uuid is a real spool identifier.
func IsValidTrayUUID(trayUUID string) bool {
	if trayUUID == "" || trayUUID == "unknown" {
		return false
	}
	if strings.ReplaceAll(trayUUID, "0", "") == "" {
		return false
	}
	return true
}

// LengthToWeight converts filament length (cm) to weight (grams) for 1.75mm filament.
func LengthToWeight(lengthCm float64, material string) float64 {
	radiusCm := 0.0875 // 1.75mm / 2 in cm
	volumeCm3 := 3.141592653589793 * radiusCm * radiusCm * lengthCm
	density := materialDensityGPerCm3["PLA"]
	if material != "" {
		if d, ok := materialDensityGPerCm3[strings.ToUpper(material)]; ok {
			density = d
		}
	}
	return volumeCm3 * density
}

// ParseExtraJSONValue parses a Spoolman extra field value stored as JSON string.
func ParseExtraJSONValue(raw string) string {
	if raw == "" {
		return ""
	}
	var parsed interface{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return raw
	}
	if s, ok := parsed.(string); ok {
		return s
	}
	return raw
}

// JSONStringifyExtraValue returns a JSON-encoded string suitable for Spoolman extra fields.
func JSONStringifyExtraValue(value string) string {
	b, _ := json.Marshal(value)
	return string(b)
}

// trayRefVariants returns entity_id / unique_id forms used for active_tray matching.
func trayRefVariants(refs ...string) []string {
	seen := make(map[string]bool)
	var out []string
	var add func(string)
	add = func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
		if strings.HasPrefix(s, "sensor.") {
			add(strings.TrimPrefix(s, "sensor."))
		}
	}
	for _, ref := range refs {
		add(ref)
	}
	return out
}

// activeTrayMatches reports whether a Spoolman active_tray value matches tray references.
func activeTrayMatches(storedRaw interface{}, storedParsed string, refs ...string) bool {
	if storedParsed == "" && storedRaw == nil {
		return false
	}
	variants := trayRefVariants(refs...)
	for _, variant := range variants {
		if storedParsed == variant {
			return true
		}
		if s, ok := storedRaw.(string); ok && s == JSONStringifyExtraValue(variant) {
			return true
		}
	}
	return false
}

// GetSpoolExtraString returns a string extra field from a spool.
func GetSpoolExtraString(spool *Spool, key string) string {
	if spool == nil || spool.Extra == nil {
		return ""
	}
	raw, ok := spool.Extra[key]
	if !ok {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return ParseExtraJSONValue(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return ParseExtraJSONValue(string(b))
	}
}
