package main

import (
	"log"
	"regexp"
	"strconv"
	"strings"
)

// FilamentUsageMetadata holds Moonraker file metadata for resolving per-extruder usage.
type FilamentUsageMetadata struct {
	FilamentWeights     []float64
	FilamentWeightTotal float64
	ReferencedTools     []int
	ExtrudersUsed       []bool // Snapmaker U1 print_task_config.extruders_used
	Size                int64
}

// FilamentUsageResolution is the deterministic result of resolving filament usage.
type FilamentUsageResolution struct {
	Usage  map[int]float64
	Source string
}

// ParseGcodeFilamentUsage extracts filament usage in grams per extruder index from G-code content.
// Extruder index matches FilaBridge toolhead index (0-based, as reported by the slicer).
func ParseGcodeFilamentUsage(gcodeContent []byte) (map[int]float64, error) {
	resolution := ResolveFilamentUsage(gcodeContent, nil)
	return resolution.Usage, nil
}

// ResolveFilamentUsage resolves filament usage deterministically from G-code and optional Moonraker metadata.
// Priority: G-code comma-separated > G-code total/single > Moonraker filament_weights >
// Moonraker referenced_tools + total > Moonraker total on extruder 0.
func ResolveFilamentUsage(gcodeContent []byte, metadata *FilamentUsageMetadata) FilamentUsageResolution {
	content := string(gcodeContent)

	if usage, source := parseGcodeCommaFilamentUsage(content); len(usage) > 0 {
		if source == "gcode_single_extruder" {
			if corrected, ok := applySnapmakerExtrudersUsed(usage, metadata); ok {
				return FilamentUsageResolution{Usage: corrected, Source: "snapmaker_extruders_used"}
			}
		}
		return FilamentUsageResolution{Usage: usage, Source: source}
	}

	if usage, source := parseGcodeTotalFilamentUsage(content); len(usage) > 0 {
		return FilamentUsageResolution{Usage: usage, Source: source}
	}

	if metadata == nil {
		return FilamentUsageResolution{Usage: map[int]float64{}, Source: ""}
	}

	if usage := usageFromFilamentWeights(metadata.FilamentWeights); len(usage) > 0 {
		return FilamentUsageResolution{Usage: usage, Source: "moonraker_filament_weights"}
	}

	if usage := usageFromReferencedTools(metadata.ReferencedTools, metadata.FilamentWeightTotal); len(usage) > 0 {
		return FilamentUsageResolution{Usage: usage, Source: "moonraker_referenced_tools"}
	}

	if metadata.FilamentWeightTotal > 0 {
		return FilamentUsageResolution{
			Usage:  map[int]float64{0: metadata.FilamentWeightTotal},
			Source: "moonraker_weight_total",
		}
	}

	return FilamentUsageResolution{Usage: map[int]float64{}, Source: ""}
}

func parseGcodeCommaFilamentUsage(content string) (map[int]float64, string) {
	gcodeRegex := regexp.MustCompile(`(?im);?\s*filament used \[g\]\s*=\s*([0-9.,\s]+)`)
	match := gcodeRegex.FindStringSubmatch(content)
	if len(match) < 2 {
		return nil, ""
	}

	usage := parseCommaSeparatedWeights(match[1])
	if len(usage) == 0 {
		return nil, ""
	}

	source := "gcode_comma"
	if !strings.Contains(match[1], ",") {
		source = "gcode_single_extruder"
	}
	return usage, source
}

func parseGcodeTotalFilamentUsage(content string) (map[int]float64, string) {
	totalRegex := regexp.MustCompile(`(?im);?\s*total filament used \[g\]\s*=\s*([0-9.]+)`)
	if match := totalRegex.FindStringSubmatch(content); len(match) >= 2 {
		if weight, err := strconv.ParseFloat(strings.TrimSpace(match[1]), 64); err == nil && weight > 0 {
			return map[int]float64{0: weight}, "gcode_total"
		}
	}
	return nil, ""
}

func parseCommaSeparatedWeights(weightsStr string) map[int]float64 {
	filamentUsage := make(map[int]float64)
	weights := strings.Split(weightsStr, ",")

	for i, weightStr := range weights {
		weightStr = strings.TrimSpace(weightStr)
		if weight, err := strconv.ParseFloat(weightStr, 64); err == nil && weight > 0 {
			filamentUsage[i] = weight
		}
	}

	return filamentUsage
}

func usageFromFilamentWeights(weights []float64) map[int]float64 {
	if len(weights) == 0 {
		return nil
	}

	usage := make(map[int]float64)
	for i, weight := range weights {
		if weight > 0 {
			usage[i] = weight
		}
	}
	return usage
}

func soleActiveExtruder(extrudersUsed []bool) (int, bool) {
	active := -1
	count := 0
	for i, used := range extrudersUsed {
		if used {
			active = i
			count++
		}
	}
	if count == 1 && active >= 0 {
		return active, true
	}
	return 0, false
}

// applySnapmakerExtrudersUsed remaps single-value G-code usage when Snapmaker firmware
// reports exactly one active extruder (print_task_config.extruders_used).
func applySnapmakerExtrudersUsed(usage map[int]float64, metadata *FilamentUsageMetadata) (map[int]float64, bool) {
	if metadata == nil || len(usage) != 1 {
		return nil, false
	}
	var weight float64
	for id, w := range usage {
		if id != 0 || w <= 0 {
			return nil, false
		}
		weight = w
	}
	extruder, ok := soleActiveExtruder(metadata.ExtrudersUsed)
	if !ok || extruder == 0 {
		return nil, false
	}
	return map[int]float64{extruder: weight}, true
}

func usageFromReferencedTools(tools []int, totalWeight float64) map[int]float64 {
	if totalWeight <= 0 || len(tools) == 0 {
		return nil
	}

	if len(tools) == 1 {
		return map[int]float64{tools[0]: totalWeight}
	}

	usage := make(map[int]float64)
	perTool := totalWeight / float64(len(tools))
	for _, tool := range tools {
		if tool >= 0 {
			usage[tool] = perTool
		}
	}
	if len(usage) == 0 {
		return nil
	}
	return usage
}

func logFilamentUsageResolution(filename, source string, usage map[int]float64) {
	if source == "" {
		return
	}
	log.Printf("Resolved filament usage for %s (source=%s): %v", filename, source, usage)
}
