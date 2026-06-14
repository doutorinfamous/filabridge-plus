package snapmaker

import (
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// FilamentUsageMetadata holds Moonraker file metadata for resolving per-extruder usage.
type FilamentUsageMetadata struct {
	FilamentWeights         []float64
	FilamentWeightTotal     float64
	ReferencedTools         []int
	ExtruderMapTable        []int  // Snapmaker U1 print_task_config.extruder_map_table (logical -> physical)
	ReprintExtruderMapTable []int  // Snapmaker U1 reprint_info.extruder_map_table
	ExtrudersUsed           []bool // Snapmaker U1 print_task_config.extruders_used
	ReprintExtrudersUsed    []bool // Snapmaker U1 reprint_info.extruders_used
	Size                    int64
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
// G-code logical extruder indices may be remapped to physical toolheads via Snapmaker print_task_config.
func ResolveFilamentUsage(gcodeContent []byte, metadata *FilamentUsageMetadata) FilamentUsageResolution {
	content := string(gcodeContent)

	if usage, source := parseGcodeCommaFilamentUsage(content); len(usage) > 0 {
		if remapped, remapSource, ok := remapSnapmakerExtruderUsage(usage, metadata); ok {
			return FilamentUsageResolution{Usage: remapped, Source: remapSource}
		}
		return FilamentUsageResolution{Usage: usage, Source: source}
	}

	if usage, source := parseGcodeTotalFilamentUsage(content); len(usage) > 0 {
		if remapped, remapSource, ok := remapSnapmakerExtruderUsage(usage, metadata); ok {
			return FilamentUsageResolution{Usage: remapped, Source: remapSource}
		}
		return FilamentUsageResolution{Usage: usage, Source: source}
	}

	if metadata == nil {
		return FilamentUsageResolution{Usage: map[int]float64{}, Source: ""}
	}

	if usage := usageFromFilamentWeights(metadata.FilamentWeights); len(usage) > 0 {
		if remapped, remapSource, ok := remapSnapmakerExtruderUsage(usage, metadata); ok {
			return FilamentUsageResolution{Usage: remapped, Source: remapSource}
		}
		return FilamentUsageResolution{Usage: usage, Source: "moonraker_filament_weights"}
	}

	if usage := usageFromReferencedTools(metadata.ReferencedTools, metadata.FilamentWeightTotal); len(usage) > 0 {
		return FilamentUsageResolution{Usage: usage, Source: "moonraker_referenced_tools"}
	}

	if metadata.FilamentWeightTotal > 0 {
		usage := map[int]float64{0: metadata.FilamentWeightTotal}
		if remapped, remapSource, ok := remapSnapmakerExtruderUsage(usage, metadata); ok {
			return FilamentUsageResolution{Usage: remapped, Source: remapSource}
		}
		return FilamentUsageResolution{
			Usage:  usage,
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

// remapSnapmakerExtruderUsage remaps logical slicer extruder indices to physical toolheads
// using Snapmaker print_task_config (extruder_map_table, then extruders_used fallback).
func remapSnapmakerExtruderUsage(logical map[int]float64, metadata *FilamentUsageMetadata) (physical map[int]float64, source string, ok bool) {
	if metadata == nil || len(logical) == 0 {
		return nil, "", false
	}

	mapTable, extrudersUsed, mapSource := selectExtruderMapTable(
		logical,
		metadata.ExtruderMapTable,
		metadata.ReprintExtruderMapTable,
		metadata.ExtrudersUsed,
		metadata.ReprintExtrudersUsed,
	)
	if len(extrudersUsed) == 0 {
		extrudersUsed = extrudersUsedForSource("", metadata.ExtrudersUsed, metadata.ReprintExtrudersUsed)
		if mapSource == "" {
			if countTrue(metadata.ReprintExtrudersUsed) > 0 && countTrue(metadata.ExtrudersUsed) == 0 {
				mapSource = "reprint_info"
			} else {
				mapSource = "main"
			}
		}
	}

	if len(mapTable) > 0 {
		if remapped, changed := remapViaExtruderMapTable(logical, mapTable); changed {
			log.Printf("Snapmaker extruder remap (extruder_map_table from %s): before=%v after=%v", mapSource, logical, remapped)
			return remapped, "snapmaker_extruder_map:" + mapSource, true
		}
	}

	if len(extrudersUsed) > 0 {
		if remapped, changed := remapViaExtrudersUsed(logical, extrudersUsed); changed {
			log.Printf("Snapmaker extruder remap (extruders_used from %s): before=%v after=%v", mapSource, logical, remapped)
			return remapped, "snapmaker_extruders_used:" + mapSource, true
		}
	}

	return nil, "", false
}

// selectExtruderMapTable picks the best extruder_map_table for logical G-code usage.
func selectExtruderMapTable(
	logical map[int]float64,
	mainTable, reprintTable []int,
	mainUsed, reprintUsed []bool,
) (table []int, extrudersUsed []bool, source string) {
	type candidate struct {
		table  []int
		used   []bool
		source string
		score  int
	}

	var candidates []candidate
	if len(mainTable) > 0 {
		candidates = append(candidates, candidate{
			table:  mainTable,
			used:   mainUsed,
			source: "main",
			score:  scoreExtruderMapTable(logical, mainTable),
		})
	}
	if len(reprintTable) > 0 {
		candidates = append(candidates, candidate{
			table:  reprintTable,
			used:   reprintUsed,
			source: "reprint_info",
			score:  scoreExtruderMapTable(logical, reprintTable),
		})
	}
	if len(candidates) == 0 {
		return nil, nil, ""
	}

	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.score > best.score {
			best = c
			continue
		}
		if c.score == best.score && c.source == "reprint_info" &&
			countTrue(mainUsed) == 0 && countTrue(reprintUsed) > 0 {
			best = c
		}
	}

	return best.table, extrudersUsedForSource(best.source, mainUsed, reprintUsed), best.source
}

func extrudersUsedForSource(source string, mainUsed, reprintUsed []bool) []bool {
	if source == "reprint_info" && countTrue(reprintUsed) > 0 {
		return reprintUsed
	}
	if source == "main" && countTrue(mainUsed) > 0 {
		return mainUsed
	}
	if countTrue(mainUsed) > 0 {
		return mainUsed
	}
	if countTrue(reprintUsed) > 0 {
		return reprintUsed
	}
	return nil
}

// scoreExtruderMapTable scores how well a map table distributes logical usage across physical extruders.
func scoreExtruderMapTable(logical map[int]float64, table []int) int {
	if len(table) == 0 {
		return -100000
	}

	score := 0
	physicalToLogical := make(map[int][]int)

	for logicalIdx, weight := range logical {
		if weight <= 0 {
			continue
		}
		if logicalIdx >= len(table) {
			return -100000
		}
		physicalIdx := table[logicalIdx]
		physicalToLogical[physicalIdx] = append(physicalToLogical[physicalIdx], logicalIdx)
	}

	for _, logicalIndices := range physicalToLogical {
		if len(logicalIndices) > 1 {
			score -= 100 * (len(logicalIndices) - 1)
		}
	}

	score += len(physicalToLogical) * 10
	return score
}

func remapViaExtruderMapTable(logical map[int]float64, mapTable []int) (map[int]float64, bool) {
	physical := make(map[int]float64)
	changed := false

	for logicalIdx, weight := range logical {
		if logicalIdx >= len(mapTable) {
			log.Printf("Warning: logical extruder %d out of extruder_map_table range (%d entries)", logicalIdx, len(mapTable))
			continue
		}
		physicalIdx := mapTable[logicalIdx]
		physical[physicalIdx] += weight
		if physicalIdx != logicalIdx {
			changed = true
		}
	}

	if len(physical) == 0 {
		return nil, false
	}
	return physical, changed
}

func remapViaExtrudersUsed(logical map[int]float64, extrudersUsed []bool) (map[int]float64, bool) {
	logicalIndices := sortedUsageIndices(logical)
	physicalIndices := sortedActiveExtruderIndices(extrudersUsed)

	if len(logicalIndices) == 0 || len(physicalIndices) == 0 {
		return nil, false
	}

	if len(logicalIndices) != len(physicalIndices) {
		log.Printf("Warning: cannot pair G-code extruders %v with extruders_used %v (count mismatch)", logicalIndices, physicalIndices)
		return nil, false
	}

	aligned := true
	for i, logicalIdx := range logicalIndices {
		if logicalIdx != physicalIndices[i] {
			aligned = false
			break
		}
	}
	if aligned {
		return nil, false
	}

	physical := make(map[int]float64, len(logicalIndices))
	for i, logicalIdx := range logicalIndices {
		physical[physicalIndices[i]] = logical[logicalIdx]
	}
	return physical, true
}

func sortedUsageIndices(usage map[int]float64) []int {
	indices := make([]int, 0, len(usage))
	for i := range usage {
		indices = append(indices, i)
	}
	sort.Ints(indices)
	return indices
}

func sortedActiveExtruderIndices(extrudersUsed []bool) []int {
	indices := make([]int, 0)
	for i, used := range extrudersUsed {
		if used {
			indices = append(indices, i)
		}
	}
	return indices
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

func distributeFilamentProportionally(actualTotalG float64, gcodeUsage map[int]float64) map[int]float64 {
	var gcodeTotal float64
	for _, weight := range gcodeUsage {
		gcodeTotal += weight
	}
	if gcodeTotal <= 0 {
		return map[int]float64{0: actualTotalG}
	}

	result := make(map[int]float64, len(gcodeUsage))
	for extruder, weight := range gcodeUsage {
		result[extruder] = actualTotalG * (weight / gcodeTotal)
	}
	return result
}

func scaleFilamentUsage(usage map[int]float64, factor float64) map[int]float64 {
	result := make(map[int]float64, len(usage))
	for extruder, weight := range usage {
		if weight > 0 {
			result[extruder] = weight * factor
		}
	}
	return result
}

func clampUnitInterval(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func sumFilamentUsage(usage map[int]float64) float64 {
	var total float64
	for _, weight := range usage {
		total += weight
	}
	return total
}
