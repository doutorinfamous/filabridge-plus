package core

import (
	"database/sql"
	"fmt"
	"time"
)

// PrintJobUsage is an aggregated filament usage entry for a print job,
// grouped by spool and slot (Moonraker toolhead or Bambu AMS tray).
type PrintJobUsage struct {
	SpoolID      int     `json:"spool_id"`
	ToolheadID   *int    `json:"toolhead_id,omitempty"`
	TrayUniqueID string  `json:"tray_unique_id,omitempty"`
	SlotName     string  `json:"slot_name"`
	Grams        float64 `json:"grams"`
}

// PrintJobRecord is a print job with aggregated filament usage.
type PrintJobRecord struct {
	ID          int64           `json:"id"`
	PrinterID   string          `json:"printer_id"`
	PrinterName string          `json:"printer_name"`
	JobName     string          `json:"job_name"`
	StartedAt   *time.Time      `json:"started_at,omitempty"`
	FinishedAt  *time.Time      `json:"finished_at,omitempty"`
	Status      string          `json:"status"`
	TotalGrams  float64         `json:"total_grams"`
	Usage       []PrintJobUsage `json:"usage"`
}

// GetPrintJobs returns print jobs (newest first) with aggregated usage.
// printerID filters by printer when non-empty. Returns jobs and total count.
func (b *FilamentBridge) GetPrintJobs(printerID string, limit, offset int) ([]PrintJobRecord, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	where := ""
	args := []interface{}{}
	if printerID != "" {
		where = "WHERE j.printer_id = ?"
		args = append(args, printerID)
	}

	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM print_jobs j %s", where)
	if err := b.DB.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count print jobs: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT j.id, j.printer_id, COALESCE(pc.name, j.printer_id), j.job_name,
		       j.started_at, j.finished_at, j.status
		FROM print_jobs j
		LEFT JOIN printer_configs pc ON pc.printer_id = j.printer_id
		%s
		ORDER BY COALESCE(j.finished_at, j.started_at) DESC, j.id DESC
		LIMIT ? OFFSET ?
	`, where)
	args = append(args, limit, offset)

	rows, err := b.DB.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query print jobs: %w", err)
	}
	defer rows.Close()

	jobs := make([]PrintJobRecord, 0)
	jobIndex := make(map[int64]int)
	for rows.Next() {
		var job PrintJobRecord
		var started, finished sql.NullTime
		if err := rows.Scan(&job.ID, &job.PrinterID, &job.PrinterName, &job.JobName, &started, &finished, &job.Status); err != nil {
			return nil, 0, fmt.Errorf("failed to scan print job: %w", err)
		}
		if started.Valid {
			t := started.Time
			job.StartedAt = &t
		}
		if finished.Valid {
			t := finished.Time
			job.FinishedAt = &t
		}
		job.Usage = make([]PrintJobUsage, 0)
		jobIndex[job.ID] = len(jobs)
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	if len(jobs) == 0 {
		return jobs, total, nil
	}

	if err := b.attachJobUsage(jobs, jobIndex); err != nil {
		return nil, 0, err
	}

	return jobs, total, nil
}

// GetPrintJob returns a single print job with aggregated usage, or nil when not found.
func (b *FilamentBridge) GetPrintJob(jobID int64) (*PrintJobRecord, error) {
	var job PrintJobRecord
	var started, finished sql.NullTime
	err := b.DB.QueryRow(`
		SELECT j.id, j.printer_id, COALESCE(pc.name, j.printer_id), j.job_name,
		       j.started_at, j.finished_at, j.status
		FROM print_jobs j
		LEFT JOIN printer_configs pc ON pc.printer_id = j.printer_id
		WHERE j.id = ?
	`, jobID).Scan(&job.ID, &job.PrinterID, &job.PrinterName, &job.JobName, &started, &finished, &job.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query print job: %w", err)
	}
	if started.Valid {
		t := started.Time
		job.StartedAt = &t
	}
	if finished.Valid {
		t := finished.Time
		job.FinishedAt = &t
	}
	job.Usage = make([]PrintJobUsage, 0)

	jobs := []PrintJobRecord{job}
	if err := b.attachJobUsage(jobs, map[int64]int{job.ID: 0}); err != nil {
		return nil, err
	}
	return &jobs[0], nil
}

// attachJobUsage aggregates filament_usage per (job, slot, spool) and fills
// Usage and TotalGrams on each job. Slot names resolve to the toolhead display
// name or the Bambu tray display name.
func (b *FilamentBridge) attachJobUsage(jobs []PrintJobRecord, jobIndex map[int64]int) error {
	ids := make([]interface{}, 0, len(jobs))
	placeholders := ""
	for i, job := range jobs {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		ids = append(ids, job.ID)
	}

	query := fmt.Sprintf(`
		SELECT u.job_id, u.toolhead_id, u.tray_unique_id, u.spool_id, SUM(u.grams),
		       COALESCE(bt.display_name, ''),
		       COALESCE(tm.display_name, '')
		FROM filament_usage u
		LEFT JOIN printer_slots bt ON bt.slot_id = u.tray_unique_id
		LEFT JOIN printer_slots tm ON tm.printer_id = u.printer_id AND tm.toolhead_id = u.toolhead_id AND tm.slot_type = '%s'
		WHERE u.job_id IN (%s)
		GROUP BY u.job_id, u.toolhead_id, u.tray_unique_id, u.spool_id
		ORDER BY u.job_id, u.toolhead_id, u.tray_unique_id
	`, SlotTypeToolhead, placeholders)

	rows, err := b.DB.Query(query, ids...)
	if err != nil {
		return fmt.Errorf("failed to query filament usage: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var jobID int64
		var toolheadID sql.NullInt64
		var trayUniqueID sql.NullString
		var spoolID int
		var grams float64
		var trayName, toolheadName string
		if err := rows.Scan(&jobID, &toolheadID, &trayUniqueID, &spoolID, &grams, &trayName, &toolheadName); err != nil {
			return fmt.Errorf("failed to scan filament usage: %w", err)
		}

		usage := PrintJobUsage{SpoolID: spoolID, Grams: grams}
		switch {
		case trayUniqueID.Valid && trayUniqueID.String != "":
			usage.TrayUniqueID = trayUniqueID.String
			usage.SlotName = trayName
			if usage.SlotName == "" {
				usage.SlotName = trayUniqueID.String
			}
		case toolheadID.Valid:
			id := int(toolheadID.Int64)
			usage.ToolheadID = &id
			usage.SlotName = toolheadName
			if usage.SlotName == "" {
				usage.SlotName = DefaultToolheadDisplayName(id)
			}
		default:
			usage.SlotName = "—"
		}

		idx, ok := jobIndex[jobID]
		if !ok {
			continue
		}
		jobs[idx].Usage = append(jobs[idx].Usage, usage)
		jobs[idx].TotalGrams += grams
	}

	return rows.Err()
}
