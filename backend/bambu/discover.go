package bambu

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"filabridge/core"
	"filabridge/homeassistant"
)

var trayKeyRegex = regexp.MustCompile(`^tray_[1-4]$`)

func getEffectiveTranslationKey(entity homeassistant.EntityRegistryEntry) string {
	if entity.TranslationKey != "" {
		if entity.TranslationKey == "tray" {
			if n, ok := entity.TranslationPlaceholders["tray_number"]; ok && n != "" {
				return "tray_" + n
			}
			if m := regexp.MustCompile(`_tray_(\d+)$`).FindStringSubmatch(entity.UniqueID); len(m) == 2 {
				return "tray_" + m[1]
			}
		}
		return entity.TranslationKey
	}
	knownKeys := []string{
		"print_status", "print_weight", "print_progress", "print_length",
		"remaining_time", "start_time", "current_layer", "total_layer", "total_layers",
		"subtask_name", "task_name", "gcode_file",
		"tray_1", "tray_2", "tray_3", "tray_4",
		"external_spool", "humidity_index", "active_tray", "stage",
	}
	for _, key := range knownKeys {
		if stringsHasSuffix(entity.UniqueID, "_"+key) {
			return key
		}
	}
	return ""
}

func stringsHasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func pickBestHAEntity(candidates []homeassistant.EntityRegistryEntry, stateMap map[string]homeassistant.State) *homeassistant.EntityRegistryEntry {
	if len(candidates) == 0 {
		return nil
	}
	if len(candidates) == 1 {
		return &candidates[0]
	}
	best := candidates[0]
	for _, current := range candidates[1:] {
		bestState := stateMap[best.EntityID]
		currentState := stateMap[current.EntityID]
		bestAvail := bestState.State != "" && bestState.State != "unavailable" && bestState.State != "unknown"
		currentAvail := currentState.State != "" && currentState.State != "unavailable" && currentState.State != "unknown"
		if currentAvail && !bestAvail {
			best = current
			continue
		}
		if bestAvail && !currentAvail {
			continue
		}
		bestSuffix := regexp.MustCompile(`_\d+$`).MatchString(best.EntityID)
		currentSuffix := regexp.MustCompile(`_\d+$`).MatchString(current.EntityID)
		if !currentSuffix && bestSuffix {
			best = current
		}
	}
	return &best
}

func parseAmsNumber(device homeassistant.DeviceRegistryEntry) int {
	m := regexp.MustCompile(`_AMS_(\d+)$`).FindStringSubmatch(device.Name)
	if len(m) == 2 {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 1
}

func getExternalSpoolIndex(uniqueID string) int {
	m := regexp.MustCompile(`(?i)_ExternalSpool(\d*)`).FindStringSubmatch(uniqueID)
	if len(m) < 2 {
		return 1
	}
	if m[1] == "" {
		return 1
	}
	n, _ := strconv.Atoi(m[1])
	if n == 0 {
		return 1
	}
	return n
}

func attrString(attrs map[string]interface{}, key string) string {
	if attrs == nil {
		return ""
	}
	v, ok := attrs[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprintf("%v", t)
	}
}

func attrFloat(attrs map[string]interface{}, key string) float64 {
	if attrs == nil {
		return 0
	}
	v, ok := attrs[key]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	default:
		return 0
	}
}

// FormatTrayDisplayName builds the canonical display name of an AMS tray or external spool.
func FormatTrayDisplayName(printerName string, amsNumber, trayNumber int, isExternal bool) string {
	if isExternal {
		return fmt.Sprintf("%s - External Spool", printerName)
	}
	amsLabel := fmt.Sprintf("AMS %d", amsNumber)
	if amsNumber >= 128 {
		htNum := amsNumber - 127
		if htNum > 1 {
			amsLabel = fmt.Sprintf("AMS HT %d", htNum)
		} else {
			amsLabel = "AMS HT"
		}
	}
	return fmt.Sprintf("%s - %s Slot %d", printerName, amsLabel, trayNumber)
}

// normalizeBambuState maps ha-bambulab print_status values to FilaBridge printer states.
func normalizeBambuState(haState string) string {
	switch strings.ToLower(strings.TrimSpace(haState)) {
	case "running", "pause", "paused":
		return core.StatePrinting
	case "idle", "finish", "finished":
		return core.StateIdle
	case "unavailable", "unknown", "":
		return core.StateOffline
	default:
		return strings.ToUpper(haState)
	}
}

func haSensorStateValue(stateMap map[string]homeassistant.State, entityID string) string {
	if entityID == "" {
		return ""
	}
	st, ok := stateMap[entityID]
	if !ok {
		return ""
	}
	value := strings.TrimSpace(st.State)
	if value == "" || value == "unknown" || value == "unavailable" {
		return ""
	}
	return value
}

func findTaskNameEntity(findEntity func(string) string) string {
	if entityID := findEntity("subtask_name"); entityID != "" {
		return entityID
	}
	return findEntity("task_name")
}

func resolveBambuJobName(stateMap map[string]homeassistant.State, taskNameEntity, gcodeFileEntity string) string {
	if name := haSensorStateValue(stateMap, taskNameEntity); name != "" {
		return name
	}
	if gcode := haSensorStateValue(stateMap, gcodeFileEntity); gcode != "" {
		return filepath.Base(gcode)
	}
	return ""
}

func parseHAProgressPercent(state string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(state), 64)
	if err != nil {
		return 0
	}
	if v > 1 {
		v = v / 100
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func parseHAStartTimeElapsed(startState homeassistant.State) float64 {
	if startState.EntityID == "" {
		return 0
	}
	raw := strings.TrimSpace(startState.State)
	if raw == "" || raw == "unavailable" || raw == "unknown" {
		return 0
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return time.Since(t).Seconds()
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil && f > 1e9 {
		return time.Since(time.Unix(int64(f), 0)).Seconds()
	}
	return 0
}

func enrichBambuPrintJob(printer *Printer, stateMap map[string]homeassistant.State, printStatus homeassistant.State, findEntity func(string) string) {
	printer.State = normalizeBambuState(printStatus.State)
	if printer.State != core.StatePrinting {
		return
	}

	taskNameEntity := findTaskNameEntity(findEntity)
	gcodeFileEntity := findEntity("gcode_file")
	printer.TaskNameEntity = taskNameEntity
	printer.GcodeFileEntity = gcodeFileEntity
	printer.JobName = resolveBambuJobName(stateMap, taskNameEntity, gcodeFileEntity)

	if entityID := findEntity("print_progress"); entityID != "" {
		printer.Progress = parseHAProgressPercent(stateMap[entityID].State)
	}

	if entityID := findEntity("start_time"); entityID != "" {
		printer.PrintDuration = parseHAStartTimeElapsed(stateMap[entityID])
	}

	if entityID := findEntity("remaining_time"); entityID != "" {
		mins, err := strconv.ParseFloat(strings.TrimSpace(stateMap[entityID].State), 64)
		if err == nil && mins >= 0 {
			secs := mins * 60
			printer.TimeRemaining = &secs
		}
	}

	if entityID := findEntity("current_layer"); entityID != "" {
		layer, err := strconv.Atoi(strings.TrimSpace(stateMap[entityID].State))
		if err == nil && layer > 0 {
			printer.CurrentLayer = &layer
		}
	}

	totalEntity := findEntity("total_layer")
	if totalEntity == "" {
		totalEntity = findEntity("total_layers")
	}
	if totalEntity != "" {
		layer, err := strconv.Atoi(strings.TrimSpace(stateMap[totalEntity].State))
		if err == nil && layer > 0 {
			printer.TotalLayer = &layer
		}
	}
}

// PrinterToPrinterData converts a Printer snapshot to core.PrinterData for WebSocket/status.
func PrinterToPrinterData(printer Printer) core.PrinterData {
	data := core.PrinterData{
		Name:  printer.Name,
		State: printer.State,
	}
	if printer.State != core.StatePrinting {
		return data
	}
	data.JobName = printer.JobName
	data.Progress = printer.Progress
	data.PrintDuration = printer.PrintDuration
	data.TimeRemaining = printer.TimeRemaining
	data.CurrentLayer = printer.CurrentLayer
	data.TotalLayer = printer.TotalLayer
	return data
}

// DiscoverPrinters discovers Bambu Lab printers from ha-bambulab via HA registries.
func DiscoverPrinters(ha *homeassistant.Client) ([]Printer, error) {
	entities, devices, err := ha.GetEntityAndDeviceRegistry()
	if err != nil {
		return nil, err
	}
	states, err := ha.GetStates()
	if err != nil {
		return nil, err
	}
	stateMap := make(map[string]homeassistant.State, len(states))
	for _, s := range states {
		stateMap[s.EntityID] = s
	}

	deviceEntityMap := make(map[string][]homeassistant.EntityRegistryEntry)
	var bambuEntities []homeassistant.EntityRegistryEntry
	for _, entity := range entities {
		if entity.Platform != "bambu_lab" || entity.DisabledBy != nil {
			continue
		}
		bambuEntities = append(bambuEntities, entity)
		if entity.DeviceID != "" {
			deviceEntityMap[entity.DeviceID] = append(deviceEntityMap[entity.DeviceID], entity)
		}
	}

	var printerEntities []homeassistant.EntityRegistryEntry
	for _, entity := range bambuEntities {
		if getEffectiveTranslationKey(entity) == "print_status" {
			printerEntities = append(printerEntities, entity)
		}
	}

	seenDevices := make(map[string]bool)
	var printers []Printer

	for _, printerEntity := range printerEntities {
		if printerEntity.DeviceID == "" || seenDevices[printerEntity.DeviceID] {
			continue
		}

		var candidates []homeassistant.EntityRegistryEntry
		for _, e := range printerEntities {
			if e.DeviceID == printerEntity.DeviceID {
				candidates = append(candidates, e)
			}
		}
		best := pickBestHAEntity(candidates, stateMap)
		if best == nil {
			continue
		}
		seenDevices[best.DeviceID] = true

		printerState := stateMap[best.EntityID]
		var printerDevice *homeassistant.DeviceRegistryEntry
		for i := range devices {
			if devices[i].ID == best.DeviceID {
				printerDevice = &devices[i]
				break
			}
		}

		prefix := best.UniqueID
		if stringsHasSuffix(prefix, "_print_status") {
			prefix = prefix[:len(prefix)-len("_print_status")]
		}
		prefix = NormalizePrefix(prefix)
		name := prefix
		if printerDevice != nil {
			if printerDevice.NameByUser != "" {
				name = printerDevice.NameByUser
			} else if printerDevice.Name != "" {
				name = printerDevice.Name
			}
		}

		var amsUnits []AMS
		var externalSpools []Tray

		for _, childDevice := range devices {
			if childDevice.ViaDeviceID != best.DeviceID {
				continue
			}
			childEntities := deviceEntityMap[childDevice.ID]

			var trayEntities []homeassistant.EntityRegistryEntry
			var extEntities []homeassistant.EntityRegistryEntry
			for _, e := range childEntities {
				key := getEffectiveTranslationKey(e)
				if trayKeyRegex.MatchString(key) {
					trayEntities = append(trayEntities, e)
				}
				if key == "external_spool" {
					extEntities = append(extEntities, e)
				}
			}

			if len(trayEntities) > 0 {
				amsNumber := parseAmsNumber(childDevice)
				amsName := fmt.Sprintf("AMS %d", amsNumber)
				if amsNumber >= 128 {
					amsName = "AMS HT"
				}
				var humidityEntity string
				for _, e := range childEntities {
					if getEffectiveTranslationKey(e) == "humidity_index" {
						humidityEntity = e.EntityID
						break
					}
				}
				ams := AMS{
					EntityID:  humidityEntity,
					Name:      amsName,
					AMSNumber: amsNumber,
				}
				seenTrayKeys := make(map[string]bool)
				for _, trayEntity := range trayEntities {
					key := getEffectiveTranslationKey(trayEntity)
					if seenTrayKeys[key] {
						continue
					}
					var sameTray []homeassistant.EntityRegistryEntry
					for _, e := range trayEntities {
						if getEffectiveTranslationKey(e) == key {
							sameTray = append(sameTray, e)
						}
					}
					bestTray := pickBestHAEntity(sameTray, stateMap)
					if bestTray == nil || bestTray.EntityID != trayEntity.EntityID {
						continue
					}
					seenTrayKeys[key] = true
					trayNum, _ := strconv.Atoi(key[5:])
					trayState := stateMap[bestTray.EntityID]
					ams.Trays = append(ams.Trays, Tray{
						EntityID:        bestTray.EntityID,
						UniqueID:        bestTray.UniqueID,
						TrayNumber:      trayNum,
						AMSNumber:       amsNumber,
						Name:            attrString(trayState.Attributes, "name"),
						Color:           attrString(trayState.Attributes, "color"),
						Material:        attrString(trayState.Attributes, "type"),
						TrayUUID:        attrString(trayState.Attributes, "tray_uuid"),
						RemainingWeight: attrFloat(trayState.Attributes, "remain"),
						DisplayName:     FormatTrayDisplayName(name, amsNumber, trayNum, false),
					})
				}
				sort.Slice(ams.Trays, func(i, j int) bool { return ams.Trays[i].TrayNumber < ams.Trays[j].TrayNumber })
				amsUnits = append(amsUnits, ams)
			}

			if len(extEntities) > 0 {
				bestExt := pickBestHAEntity(extEntities, stateMap)
				if bestExt != nil {
					extState := stateMap[bestExt.EntityID]
					externalSpools = append(externalSpools, Tray{
						EntityID:        bestExt.EntityID,
						UniqueID:        bestExt.UniqueID,
						IsExternal:      true,
						Name:            attrString(extState.Attributes, "name"),
						Color:           attrString(extState.Attributes, "color"),
						Material:        attrString(extState.Attributes, "type"),
						TrayUUID:        attrString(extState.Attributes, "tray_uuid"),
						RemainingWeight: attrFloat(extState.Attributes, "remain"),
						DisplayName:     FormatTrayDisplayName(name, 0, 0, true),
					})
				}
			}
		}

		sort.Slice(amsUnits, func(i, j int) bool { return amsUnits[i].AMSNumber < amsUnits[j].AMSNumber })
		sort.Slice(externalSpools, func(i, j int) bool {
			var aUID, bUID string
			for _, e := range bambuEntities {
				if e.EntityID == externalSpools[i].EntityID {
					aUID = e.UniqueID
				}
				if e.EntityID == externalSpools[j].EntityID {
					bUID = e.UniqueID
				}
			}
			return getExternalSpoolIndex(aUID) < getExternalSpoolIndex(bUID)
		})

		printerDeviceEntities := deviceEntityMap[best.DeviceID]
		findPrinterEntity := func(key string) string {
			var candidates []homeassistant.EntityRegistryEntry
			for _, e := range printerDeviceEntities {
				if getEffectiveTranslationKey(e) == key {
					candidates = append(candidates, e)
				}
			}
			if picked := pickBestHAEntity(candidates, stateMap); picked != nil {
				return picked.EntityID
			}
			return ""
		}

		printer := Printer{
			EntityID:            best.EntityID,
			DeviceID:            best.DeviceID,
			Prefix:              prefix,
			Name:                name,
			State:               printerState.State,
			AMSUnits:            amsUnits,
			ExternalSpools:      externalSpools,
			CurrentStageEntity:  findPrinterEntity("stage"),
			TaskNameEntity:      findTaskNameEntity(findPrinterEntity),
			GcodeFileEntity:     findPrinterEntity("gcode_file"),
			PrintWeightEntity:   findPrinterEntity("print_weight"),
			PrintProgressEntity: findPrinterEntity("print_progress"),
		}
		enrichBambuPrintJob(&printer, stateMap, printerState, findPrinterEntity)
		printers = append(printers, printer)
	}

	return printers, nil
}

// CollectTrayInfos collects tray info for HA config generation.
func CollectTrayInfos(printer Printer) []TrayInfo {
	var trays []TrayInfo
	for i, ext := range printer.ExternalSpools {
		trays = append(trays, TrayInfo{
			EntityID:    ext.EntityID,
			AMSNumber:   0,
			TrayNumber:  0,
			CompositeID: i,
		})
	}
	for _, ams := range printer.AMSUnits {
		for _, tray := range ams.Trays {
			trays = append(trays, TrayInfo{
				EntityID:    tray.EntityID,
				AMSNumber:   ams.AMSNumber,
				TrayNumber:  tray.TrayNumber,
				CompositeID: ams.AMSNumber*10 + tray.TrayNumber,
			})
		}
	}
	return trays
}
