package main

import (
	"regexp"
	"strconv"
	"strings"
)

// ParseGcodeFilamentUsage extracts filament usage in grams per toolhead from G-code content.
func ParseGcodeFilamentUsage(gcodeContent []byte) (map[int]float64, error) {
	content := string(gcodeContent)
	filamentUsage := make(map[int]float64)

	// Common PrusaSlicer/Orca/Bambu-style metadata with comma-separated toolhead values.
	gcodeRegex := regexp.MustCompile(`;?\s*filament used \[g\]\s*=\s*([0-9.,\s]+)`)
	if match := gcodeRegex.FindStringSubmatch(content); len(match) >= 2 {
		if usage := parseCommaSeparatedWeights(match[1]); len(usage) > 0 {
			return usage, nil
		}
	}

	// Some slicers only expose a single total value.
	totalRegex := regexp.MustCompile(`(?i);?\s*total filament used \[g\]\s*=\s*([0-9.]+)`)
	if match := totalRegex.FindStringSubmatch(content); len(match) >= 2 {
		if weight, err := strconv.ParseFloat(strings.TrimSpace(match[1]), 64); err == nil && weight > 0 {
			filamentUsage[0] = weight
			return filamentUsage, nil
		}
	}

	// Klipper-style per-extruder comments, e.g. "; filament used [g] = 12.34" for each toolhead block.
	perExtruderRegex := regexp.MustCompile(`(?i);?\s*filament used \[g\]\s*=\s*([0-9.]+)`)
	matches := perExtruderRegex.FindAllStringSubmatch(content, -1)
	if len(matches) == 1 {
		if weight, err := strconv.ParseFloat(strings.TrimSpace(matches[0][1]), 64); err == nil && weight > 0 {
			filamentUsage[0] = weight
			return filamentUsage, nil
		}
	}

	return filamentUsage, nil
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
