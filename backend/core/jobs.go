package core

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// Print job statuses.
const (
	JobStatusPrinting  = "printing"
	JobStatusCompleted = "completed"
	JobStatusCancelled = "cancelled"
	JobStatusFailed    = "failed"
)

// StartPrintJob opens a print job for a printer. It is idempotent: if an open
// job already exists for the same printer/file it is reused. Any other open
// jobs for the printer are closed as cancelled (their end was never observed).
func (b *FilamentBridge) StartPrintJob(printerID, jobName string) (int64, error) {
	if jobName == "" {
		return 0, nil
	}

	var existingID int64
	err := b.DB.QueryRow(
		"SELECT id FROM print_jobs WHERE printer_id = ? AND job_name = ? AND status = ? ORDER BY id DESC LIMIT 1",
		printerID, jobName, JobStatusPrinting,
	).Scan(&existingID)
	if err == nil {
		return existingID, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("failed to check open print job: %w", err)
	}

	// Close stale open jobs for this printer (a new print started without us
	// ever seeing the previous one finish).
	res, err := b.DB.Exec(
		"UPDATE print_jobs SET status = ?, finished_at = ? WHERE printer_id = ? AND status = ?",
		JobStatusCancelled, time.Now(), printerID, JobStatusPrinting,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to close stale print jobs: %w", err)
	}
	if stale, _ := res.RowsAffected(); stale > 0 {
		log.Printf("Closed %d stale open print job(s) for %s as cancelled", stale, printerID)
	}

	insert, err := b.DB.Exec(
		"INSERT INTO print_jobs (printer_id, job_name, started_at, status) VALUES (?, ?, ?, ?)",
		printerID, jobName, time.Now(), JobStatusPrinting,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to start print job: %w", err)
	}
	jobID, err := insert.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get print job id: %w", err)
	}

	log.Printf("Started print job %d for %s: %s", jobID, printerID, jobName)
	return jobID, nil
}

// IsJobProcessed reports whether the most recent job for this printer/file is
// already closed (filament usage debited). Replaces the old processed_jobs table.
func (b *FilamentBridge) IsJobProcessed(printerID, filename string) (bool, error) {
	if filename == "" {
		return false, nil
	}

	var status string
	err := b.DB.QueryRow(
		"SELECT status FROM print_jobs WHERE printer_id = ? AND job_name = ? ORDER BY id DESC LIMIT 1",
		printerID, filename,
	).Scan(&status)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check processed job: %w", err)
	}
	return status != JobStatusPrinting, nil
}

// FinishPrintJob closes the open job for a printer/file with the given status.
// If no open job exists (start was missed), a closed row is created so the
// job still counts as processed.
func (b *FilamentBridge) FinishPrintJob(printerID, jobName, status string) error {
	if jobName == "" {
		return nil
	}

	res, err := b.DB.Exec(
		"UPDATE print_jobs SET status = ?, finished_at = ? WHERE printer_id = ? AND job_name = ? AND status IN (?, ?)",
		status, time.Now(), printerID, jobName, JobStatusPrinting, JobStatusFailed,
	)
	if err != nil {
		return fmt.Errorf("failed to finish print job: %w", err)
	}
	if affected, _ := res.RowsAffected(); affected > 0 {
		return nil
	}

	// No open job — record a closed one so dedup still works.
	_, err = b.DB.Exec(
		"INSERT INTO print_jobs (printer_id, job_name, finished_at, status) VALUES (?, ?, ?, ?)",
		printerID, jobName, time.Now(), status,
	)
	if err != nil {
		return fmt.Errorf("failed to record finished print job: %w", err)
	}
	return nil
}

// FinishLatestOpenJob closes the most recent open job for a printer regardless
// of file name (used by Bambu webhooks when the job name is unavailable).
func (b *FilamentBridge) FinishLatestOpenJob(printerID, status string) error {
	var jobID int64
	err := b.DB.QueryRow(
		"SELECT id FROM print_jobs WHERE printer_id = ? AND status = ? ORDER BY id DESC LIMIT 1",
		printerID, JobStatusPrinting,
	).Scan(&jobID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to find open print job: %w", err)
	}

	_, err = b.DB.Exec(
		"UPDATE print_jobs SET status = ?, finished_at = ? WHERE id = ?",
		status, time.Now(), jobID,
	)
	if err != nil {
		return fmt.Errorf("failed to finish print job: %w", err)
	}
	return nil
}

// GetOrCreateOpenJob returns the open job for a printer, creating one when
// needed. When jobName is provided, an open job with the same name is
// preferred; otherwise the latest open job for the printer is used.
func (b *FilamentBridge) GetOrCreateOpenJob(printerID, jobName string) (int64, error) {
	var jobID int64
	var err error
	if jobName != "" {
		err = b.DB.QueryRow(
			"SELECT id FROM print_jobs WHERE printer_id = ? AND job_name = ? AND status = ? ORDER BY id DESC LIMIT 1",
			printerID, jobName, JobStatusPrinting,
		).Scan(&jobID)
		if err == nil {
			return jobID, nil
		}
		if err != sql.ErrNoRows {
			return 0, fmt.Errorf("failed to find open print job: %w", err)
		}
	}

	// Fall back to any open job for the printer (e.g. usage events arriving
	// without a job name).
	err = b.DB.QueryRow(
		"SELECT id FROM print_jobs WHERE printer_id = ? AND status = ? ORDER BY id DESC LIMIT 1",
		printerID, JobStatusPrinting,
	).Scan(&jobID)
	if err == nil {
		if jobName != "" {
			// Backfill the job name when the open job was created without one.
			if _, updErr := b.DB.Exec(
				"UPDATE print_jobs SET job_name = ? WHERE id = ? AND job_name = ''",
				jobName, jobID,
			); updErr != nil {
				log.Printf("Warning: failed to backfill job name for job %d: %v", jobID, updErr)
			}
		}
		return jobID, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("failed to find open print job: %w", err)
	}

	insert, err := b.DB.Exec(
		"INSERT INTO print_jobs (printer_id, job_name, started_at, status) VALUES (?, ?, ?, ?)",
		printerID, jobName, time.Now(), JobStatusPrinting,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create print job: %w", err)
	}
	return insert.LastInsertId()
}

// GetLatestPrintJobID returns the most recent print job id for a printer/file.
func (b *FilamentBridge) GetLatestPrintJobID(printerID, jobName string) (int64, error) {
	if jobName == "" {
		return 0, fmt.Errorf("job name is required")
	}

	var jobID int64
	err := b.DB.QueryRow(
		"SELECT id FROM print_jobs WHERE printer_id = ? AND job_name = ? ORDER BY id DESC LIMIT 1",
		printerID, jobName,
	).Scan(&jobID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to find print job: %w", err)
	}
	return jobID, nil
}

// LogToolheadUsage records a filament usage event for a Moonraker toolhead.
func (b *FilamentBridge) LogToolheadUsage(jobID int64, printerID string, toolheadID, spoolID int, grams float64) error {
	_, err := b.DB.Exec(
		"INSERT INTO filament_usage (job_id, printer_id, toolhead_id, spool_id, grams, recorded_at) VALUES (?, ?, ?, ?, ?, ?)",
		jobID, printerID, toolheadID, spoolID, grams, time.Now(),
	)
	if err != nil {
		return fmt.Errorf("failed to log filament usage: %w", err)
	}
	return nil
}

// LogTrayUsage records a filament usage event for a Bambu AMS tray.
func (b *FilamentBridge) LogTrayUsage(jobID int64, printerID, trayUniqueID string, spoolID int, grams float64) error {
	_, err := b.DB.Exec(
		"INSERT INTO filament_usage (job_id, printer_id, tray_unique_id, spool_id, grams, recorded_at) VALUES (?, ?, ?, ?, ?, ?)",
		jobID, printerID, trayUniqueID, spoolID, grams, time.Now(),
	)
	if err != nil {
		return fmt.Errorf("failed to log filament usage: %w", err)
	}
	return nil
}

// ProcessFilamentUsage processes filament usage updates for all toolheads.
// Extruder index from G-code maps directly to FilaBridge toolhead index (no remapping).
func (b *FilamentBridge) ProcessFilamentUsage(printerID string, filamentUsage map[int]float64, jobName string) error {
	printerName := b.ResolvePrinterDisplayName(printerID)

	jobID, err := b.GetOrCreateOpenJob(printerID, jobName)
	if err != nil {
		log.Printf("Warning: failed to resolve print job for %s (%s): %v", printerName, jobName, err)
	}

	updatedCount := 0
	unmappedToolheads := make([]int, 0)

	for toolheadID, usedWeight := range filamentUsage {
		if usedWeight <= 0 {
			continue
		}

		spoolID, err := b.GetToolheadMapping(printerID, toolheadID)
		if err != nil {
			errMsg := fmt.Sprintf("error getting toolhead mapping for %s toolhead %d: %v", printerName, toolheadID, err)
			log.Print(errMsg)
			b.AddPrintError(PrintErrorInput{
				PrinterID:   printerID,
				PrinterName: printerName,
				JobName:     jobName,
				Error:       errMsg,
				ToolheadID:  toolheadID,
				Grams:       usedWeight,
			})
			continue
		}

		if spoolID == 0 {
			log.Printf("No spool mapped to %s toolhead %d, skipping filament usage update",
				printerName, toolheadID)
			unmappedToolheads = append(unmappedToolheads, toolheadID)
			continue
		}

		if err := b.Spoolman.UpdateSpoolUsage(spoolID, usedWeight); err != nil {
			errMsg := fmt.Sprintf("failed to update spool %d in Spoolman: %v", spoolID, err)
			log.Printf("Error updating spool %d usage: %v", spoolID, err)
			b.AddPrintError(PrintErrorInput{
				PrinterID:   printerID,
				PrinterName: printerName,
				JobName:     jobName,
				Error:       errMsg,
				ToolheadID:  toolheadID,
				Grams:       usedWeight,
			})
			continue
		}

		if err := b.LogToolheadUsage(jobID, printerID, toolheadID, spoolID, usedWeight); err != nil {
			log.Printf("Error logging print usage: %v", err)
		}

		updatedCount++
		log.Printf("Updated spool %d: used %.2fg filament on %s toolhead %d",
			spoolID, usedWeight, printerName, toolheadID)
	}

	for _, toolheadID := range unmappedToolheads {
		weight := filamentUsage[toolheadID]
		errMsg := fmt.Sprintf(
			"G-code reports %.2fg on extruder %d, but no spool is mapped to %s. Map a spool to the correct toolhead or configure the correct extruder in Snapmaker Orca.",
			weight, toolheadID, DefaultToolheadDisplayName(toolheadID),
		)
		b.AddPrintError(PrintErrorInput{
			PrinterID:   printerID,
			PrinterName: printerName,
			JobName:     jobName,
			Error:       errMsg,
			ToolheadID:  toolheadID,
			Grams:       weight,
		})
	}

	if updatedCount > 0 {
		log.Printf("Print completion processing finished for %s: updated %d spool(s)", printerName, updatedCount)
		return nil
	}

	if len(filamentUsage) > 0 {
		log.Printf("No filament usage data processed for %s", printerName)
		return fmt.Errorf("no spools were updated for print %s", jobName)
	}

	log.Printf("No filament usage data processed for %s", printerName)
	return fmt.Errorf("no filament usage to process for print %s", jobName)
}
