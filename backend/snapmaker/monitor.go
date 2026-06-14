package snapmaker

import (
	"fmt"
	"log"
	"strings"
	"time"

	"filabridge/core"
)

// shouldProcessCompletedJob detects finished prints that were missed by WasPrinting tracking.
func shouldProcessCompletedJob(rawState, filename string, printDuration float64, alreadyProcessed bool) bool {
	return strings.ToLower(strings.TrimSpace(rawState)) == MoonrakerStateComplete &&
		filename != "" &&
		printDuration > 0 &&
		!alreadyProcessed
}

// shouldProcessCancelledJob detects cancelled prints that need partial filament debit.
func shouldProcessCancelledJob(rawState, filename string, printDuration float64, alreadyProcessed bool) bool {
	return IsMoonrakerCancelledState(rawState) &&
		filename != "" &&
		printDuration > 0 &&
		!alreadyProcessed
}

// MonitorPrinters monitors all Moonraker printers for print status changes.
// Bambu printers are skipped — they report usage via Home Assistant webhooks.
func MonitorPrinters(b *core.FilamentBridge) {
	log.Printf("Monitoring printers at %s", time.Now().Format(time.RFC3339))

	configSnapshot := b.GetConfigSnapshot()
	if configSnapshot == nil || len(configSnapshot.Printers) == 0 {
		log.Printf("No printers configured - skipping monitoring")
		return
	}

	for printerID, printerConfig := range configSnapshot.Printers {
		if printerID == "no_printers" {
			continue // Skip placeholder
		}
		if printerConfig.Driver == core.DriverBambuHA {
			continue
		}
		go func(printerID string, config core.PrinterConfig) {
			if err := monitorPrinter(b, printerID, config); err != nil {
				log.Printf("Error monitoring printer %s (%s): %v", config.IPAddress, printerID, err)
			}
		}(printerID, printerConfig)
	}
}

// monitorPrinter monitors a single printer using Snapmaker U1 Moonraker API.
func monitorPrinter(b *core.FilamentBridge, printerID string, config core.PrinterConfig) error {
	log.Printf("Starting monitoring for printer %s (%s) at %s", printerID, config.IPAddress, config.Name)
	client := NewMoonrakerClient(config.IPAddress, config.APIKey, b.Config.PrinterTimeout, b.Config.PrinterFileDownloadTimeout)

	printerStatus, err := client.GetPrinterStatus()
	if err != nil {
		log.Printf("Warning: Failed to get printer status from %s (%s): %v", config.IPAddress, printerID, err)
		return nil // Don't fail the entire monitoring cycle for one printer
	}

	currentState := printerStatus.State
	rawState := printerStatus.RawState
	jobName := printerStatus.JobDisplayName
	currentJobFilename := printerStatus.JobFilename

	// Check if print just finished - minimize lock scope
	b.Mutex.RLock()
	wasPrinting := b.WasPrinting[printerID]
	storedJobFile := b.CurrentJobFile[printerID]
	b.Mutex.RUnlock()

	filenameToUse := storedJobFile
	if filenameToUse == "" {
		filenameToUse = currentJobFilename
	}

	alreadyProcessed := false
	if filenameToUse != "" {
		alreadyProcessed, err = b.IsJobProcessed(printerID, filenameToUse)
		if err != nil {
			log.Printf("Warning: Failed to check processed job for %s (%s): %v", config.IPAddress, printerID, err)
		}
	}

	missedCompletion := shouldProcessCompletedJob(rawState, filenameToUse, printerStatus.PrintDuration, alreadyProcessed)
	transitionFinish := wasPrinting && IsMoonrakerFinishedState(rawState) && rawState != MoonrakerStateError
	cancelledJob := shouldProcessCancelledJob(rawState, filenameToUse, printerStatus.PrintDuration, alreadyProcessed)

	log.Printf("Printer %s (%s): state=%s, raw_state=%s, wasPrinting=%v, job=%s, stored_file=%s, print_duration=%.1f, alreadyProcessed=%v",
		config.IPAddress, printerID, currentState, rawState, wasPrinting, jobName, storedJobFile, printerStatus.PrintDuration, alreadyProcessed)

	if transitionFinish || missedCompletion {
		if filenameToUse == "" {
			log.Printf("Warning: Print completion detected for %s (%s) but no filename available", config.IPAddress, printerID)
		} else {
			detectionReason := "transition"
			if missedCompletion && !transitionFinish {
				detectionReason = "complete-state"
			}

			log.Printf("Print finished detected for %s (%s): %s (state: %s, file: %s, reason: %s)",
				config.IPAddress, printerID, jobName, currentState, filenameToUse, detectionReason)

			b.Mutex.Lock()
			b.WasPrinting[printerID] = false
			b.ProcessingPrints[printerID] = true
			b.Mutex.Unlock()

			err := handlePrintFinished(b, printerID, config, filenameToUse, printerStatus)

			b.Mutex.Lock()
			b.ProcessingPrints[printerID] = false
			b.CurrentJobFile[printerID] = ""
			jobStatus := core.JobStatusCompleted
			if err != nil {
				jobStatus = core.JobStatusFailed
			}
			if markErr := b.FinishPrintJob(printerID, filenameToUse, jobStatus); markErr != nil {
				log.Printf("Warning: Failed to mark job as processed for %s (%s): %v", config.IPAddress, printerID, markErr)
			}
			b.Mutex.Unlock()

			if err != nil {
				log.Printf("Error handling print finished: %v", err)
			}
		}
	} else if cancelledJob {
		log.Printf("Print cancelled detected for %s (%s): %s (state: %s, file: %s)",
			config.IPAddress, printerID, jobName, currentState, filenameToUse)

		b.Mutex.Lock()
		b.WasPrinting[printerID] = false
		b.ProcessingPrints[printerID] = true
		b.Mutex.Unlock()

		err := handlePrintCancelled(b, printerID, config, filenameToUse, printerStatus)

		b.Mutex.Lock()
		b.ProcessingPrints[printerID] = false
		if err == nil {
			b.CurrentJobFile[printerID] = ""
			if markErr := b.FinishPrintJob(printerID, filenameToUse, core.JobStatusCancelled); markErr != nil {
				log.Printf("Warning: Failed to mark cancelled job as processed for %s (%s): %v", config.IPAddress, printerID, markErr)
			}
		}
		b.Mutex.Unlock()

		if err != nil {
			log.Printf("Error handling print cancelled: %v", err)
		}
	} else {
		// Update state tracking - minimize lock scope
		b.Mutex.Lock()
		defer b.Mutex.Unlock()

		// Store the current job filename when printing starts (only if not already stored)
		if IsMoonrakerPrintingState(rawState) && currentJobFilename != "" {
			if storedJobFile == "" {
				b.CurrentJobFile[printerID] = currentJobFilename
				log.Printf("Stored job filename for %s (%s): %s", config.IPAddress, printerID, currentJobFilename)
			}
			// Open the print job (idempotent) so started_at reflects the real start
			// and the previous completed run of the same file stops counting as processed.
			if _, err := b.StartPrintJob(printerID, currentJobFilename); err != nil {
				log.Printf("Warning: Failed to start print job for %s (%s): %v", config.IPAddress, printerID, err)
			}
			delete(b.PendingUsage, core.PendingUsageKey(printerID, currentJobFilename))
		}

		// Update WasPrinting flag for NEXT cycle
		b.WasPrinting[printerID] = IsMoonrakerPrintingState(rawState)

		// Clear stored filename when print finishes (but only if not currently processing)
		if IsMoonrakerFinishedState(rawState) && !b.ProcessingPrints[printerID] {
			b.CurrentJobFile[printerID] = ""
		}
	}

	return nil
}

// handlePrintFinished handles when a print job finishes via Moonraker.
func handlePrintFinished(b *core.FilamentBridge, printerID string, config core.PrinterConfig, filename string, printerStatus *PrinterStatus) error {
	log.Printf("Print finished via Moonraker (%s): %s", config.IPAddress, filename)

	printerName := core.ResolvePrinterName(config)
	cacheKey := core.PendingUsageKey(printerID, filename)

	if filename == "" {
		errorMsg := "no filename available for print processing"
		b.AddPrintError(core.PrintErrorInput{
			PrinterID:   printerID,
			PrinterName: printerName,
			JobName:     "unknown",
			Error:       errorMsg,
			ToolheadID:  -1,
		})
		return fmt.Errorf("%s", errorMsg)
	}

	var filamentUsage map[int]float64

	b.Mutex.RLock()
	cachedUsage, hasCached := b.PendingUsage[cacheKey]
	b.Mutex.RUnlock()

	if hasCached {
		filamentUsage = cachedUsage
		log.Printf("Using cached filament usage for %s: %+v", filename, filamentUsage)
	} else {
		client := NewMoonrakerClient(config.IPAddress, config.APIKey, b.Config.PrinterTimeout, b.Config.PrinterFileDownloadTimeout)

		log.Printf("Analyzing G-code file for filament usage: %s", filename)

		var err error
		filamentUsage, err = client.ParseFilamentUsageFromFile(filename, b.Config.PrinterFileDownloadTimeout)
		if err != nil {
			errorMsg := fmt.Sprintf("failed to parse G-code for filament usage: %v", err)
			b.AddPrintError(core.PrintErrorInput{
				PrinterID:   printerID,
				PrinterName: printerName,
				JobName:     filename,
				Error:       errorMsg,
				ToolheadID:  -1,
			})
			return fmt.Errorf("%s", errorMsg)
		}

		if len(filamentUsage) == 0 && printerStatus != nil && printerStatus.FilamentUsed > 0 {
			if weight := filamentUsageFromPrintStats(b, printerID, printerStatus.FilamentUsed); weight > 0 {
				log.Printf("Using print_stats.filament_used fallback: %.2fmm -> %.2fg", printerStatus.FilamentUsed, weight)
				logical := map[int]float64{0: weight}
				meta := &FilamentUsageMetadata{}
				applySnapmakerExtruderMapping(meta, client.GetPrintTaskFilamentMapping())
				if remapped, _, ok := remapSnapmakerExtruderUsage(logical, meta); ok {
					filamentUsage = remapped
				} else {
					filamentUsage = logical
				}
			}
		}

		if len(filamentUsage) == 0 {
			errorMsg := "no filament usage data found in G-code file or Moonraker print stats"
			b.AddPrintError(core.PrintErrorInput{
				PrinterID:   printerID,
				PrinterName: printerName,
				JobName:     filename,
				Error:       errorMsg,
				ToolheadID:  -1,
			})
			return fmt.Errorf("%s", errorMsg)
		}

		log.Printf("Successfully resolved filament usage: %+v", filamentUsage)

		b.Mutex.Lock()
		b.PendingUsage[cacheKey] = filamentUsage
		b.Mutex.Unlock()
	}

	if err := b.ProcessFilamentUsage(printerID, filamentUsage, filename); err != nil {
		log.Printf("Error processing filament usage: %v", err)
		return err
	}

	b.Mutex.Lock()
	delete(b.PendingUsage, cacheKey)
	b.Mutex.Unlock()

	return nil
}

type partialFilamentResolution struct {
	Usage  map[int]float64
	Source string
}

// handlePrintCancelled debits partial filament usage when a print is cancelled.
func handlePrintCancelled(b *core.FilamentBridge, printerID string, config core.PrinterConfig, filename string, printerStatus *PrinterStatus) error {
	log.Printf("Print cancelled via Moonraker (%s): %s", config.IPAddress, filename)

	printerName := core.ResolvePrinterName(config)
	if filename == "" {
		errorMsg := "no filename available for cancelled print processing"
		b.AddPrintError(core.PrintErrorInput{
			PrinterID:   printerID,
			PrinterName: printerName,
			JobName:     "unknown",
			Error:       errorMsg,
			ToolheadID:  -1,
		})
		return fmt.Errorf("%s", errorMsg)
	}

	client := NewMoonrakerClient(config.IPAddress, config.APIKey, b.Config.PrinterTimeout, b.Config.PrinterFileDownloadTimeout)
	resolution, err := resolvePartialFilamentUsage(b, client, printerID, filename, printerStatus, b.Config.PrinterFileDownloadTimeout)
	if err != nil {
		errorMsg := fmt.Sprintf("failed to resolve partial filament usage: %v", err)
		b.AddPrintError(core.PrintErrorInput{
			PrinterID:   printerID,
			PrinterName: printerName,
			JobName:     filename,
			Error:       errorMsg,
			ToolheadID:  -1,
		})
		return fmt.Errorf("%s", errorMsg)
	}

	totalG := sumFilamentUsage(resolution.Usage)
	log.Printf("Print cancelled — debited %.2fg (source: %s) for %s: %+v", totalG, resolution.Source, filename, resolution.Usage)

	if err := b.ProcessFilamentUsage(printerID, resolution.Usage, filename); err != nil {
		log.Printf("Error processing cancelled print filament usage: %v", err)
		return err
	}

	return nil
}

func resolvePartialFilamentUsage(
	b *core.FilamentBridge,
	client *MoonrakerClient,
	printerID string,
	filename string,
	printerStatus *PrinterStatus,
	fileDownloadTimeout int,
) (partialFilamentResolution, error) {
	if printerStatus != nil && printerStatus.FilamentUsed > 0 {
		actualG := filamentUsageFromPrintStats(b, printerID, printerStatus.FilamentUsed)
		if actualG > 0 {
			usage, err := resolvePartialUsageFromPrintStats(client, filename, actualG, fileDownloadTimeout)
			if err == nil && len(usage) > 0 {
				return partialFilamentResolution{Usage: usage, Source: "print_stats"}, nil
			}
		}
	}

	gcodeUsage, err := client.ParseFilamentUsageFromFile(filename, fileDownloadTimeout)
	if err != nil {
		return partialFilamentResolution{}, fmt.Errorf("failed to parse G-code for filament usage: %w", err)
	}
	if len(gcodeUsage) == 0 {
		return partialFilamentResolution{}, fmt.Errorf("no filament usage data found in G-code file")
	}

	if printerStatus != nil && printerStatus.Progress > 0 {
		factor := clampUnitInterval(printerStatus.Progress)
		return partialFilamentResolution{
			Usage:  scaleFilamentUsage(gcodeUsage, factor),
			Source: "progress",
		}, nil
	}

	if printerStatus != nil && printerStatus.PrintDuration > 0 {
		if meta, metaErr := client.GetFileMetadata(filename); metaErr == nil && meta != nil && meta.EstimatedTime > 0 {
			factor := clampUnitInterval(printerStatus.PrintDuration / meta.EstimatedTime)
			if factor > 0 {
				return partialFilamentResolution{
					Usage:  scaleFilamentUsage(gcodeUsage, factor),
					Source: "duration",
				}, nil
			}
		}
	}

	return partialFilamentResolution{}, fmt.Errorf("no partial filament usage data available")
}

func resolvePartialUsageFromPrintStats(
	client *MoonrakerClient,
	filename string,
	actualG float64,
	fileDownloadTimeout int,
) (map[int]float64, error) {
	gcodeUsage, err := client.ParseFilamentUsageFromFile(filename, fileDownloadTimeout)
	if err == nil && len(gcodeUsage) > 0 {
		usage := distributeFilamentProportionally(actualG, gcodeUsage)
		return applySnapmakerExtruderRemap(client, usage), nil
	}

	logical := map[int]float64{0: actualG}
	return applySnapmakerExtruderRemap(client, logical), nil
}

func applySnapmakerExtruderRemap(client *MoonrakerClient, usage map[int]float64) map[int]float64 {
	meta := &FilamentUsageMetadata{}
	applySnapmakerExtruderMapping(meta, client.GetPrintTaskFilamentMapping())
	if remapped, _, ok := remapSnapmakerExtruderUsage(usage, meta); ok {
		return remapped
	}
	return usage
}

// filamentUsageFromPrintStats converts Moonraker print_stats.filament_used (mm) to grams.
func filamentUsageFromPrintStats(b *core.FilamentBridge, printerID string, filamentUsedMm float64) float64 {
	const defaultDiameter = 1.75
	const defaultDensity = 1.24

	diameter := defaultDiameter
	density := defaultDensity

	spoolID, err := b.GetToolheadMapping(printerID, 0)
	if err == nil && spoolID > 0 {
		spool, spoolErr := b.Spoolman.GetSpool(spoolID)
		if spoolErr == nil && spool.Filament != nil {
			if spool.Filament.Diameter > 0 {
				diameter = spool.Filament.Diameter
			}
			if spool.Filament.Density > 0 {
				density = spool.Filament.Density
			}
		}
	}

	return FilamentLengthMmToGrams(filamentUsedMm, diameter, density)
}
