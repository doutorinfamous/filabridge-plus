package core

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// tableExists reports whether a user table exists in the SQLite database.
func (b *FilamentBridge) tableExists(name string) (bool, error) {
	var count int
	err := b.DB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?",
		name,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// tableHasColumn reports whether a table contains a column with the given name.
func (b *FilamentBridge) tableHasColumn(table, column string) (bool, error) {
	rows, err := b.DB.Query(fmt.Sprintf("PRAGMA table_info(%q)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dfltValue interface{}
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// migrateSchemaV2 rebuilds legacy tables into the printer_id based schema:
//   - toolhead_mappings: printer_name key -> printer_id key + embedded display_name
//   - toolhead_names: merged into toolhead_mappings.display_name and dropped
//   - print_history: converted into print_jobs + filament_usage and dropped
//   - processed_jobs: converted into closed print_jobs rows and dropped
func (b *FilamentBridge) migrateSchemaV2() error {
	if err := b.migrateToolheadMappingsV2(); err != nil {
		return fmt.Errorf("toolhead_mappings migration: %w", err)
	}
	if err := b.migrateToolheadNamesV2(); err != nil {
		return fmt.Errorf("toolhead_names migration: %w", err)
	}
	if err := b.migratePrintHistoryV2(); err != nil {
		return fmt.Errorf("print_history migration: %w", err)
	}
	if err := b.migrateProcessedJobsV2(); err != nil {
		return fmt.Errorf("processed_jobs migration: %w", err)
	}
	return nil
}

// migrateToolheadMappingsV2 rebuilds toolhead_mappings keyed by printer_id.
func (b *FilamentBridge) migrateToolheadMappingsV2() error {
	hasPrinterName, err := b.tableHasColumn("toolhead_mappings", "printer_name")
	if err != nil {
		return err
	}
	if !hasPrinterName {
		return nil // Already in the new shape
	}

	tx, err := b.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		CREATE TABLE toolhead_mappings_v2 (
			printer_id TEXT NOT NULL,
			toolhead_id INTEGER NOT NULL,
			display_name TEXT NOT NULL DEFAULT '',
			spool_id INTEGER,
			mapped_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (printer_id, toolhead_id)
		)
	`); err != nil {
		return err
	}

	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO toolhead_mappings_v2 (printer_id, toolhead_id, spool_id, mapped_at)
		SELECT pc.printer_id, tm.toolhead_id, tm.spool_id, tm.mapped_at
		FROM toolhead_mappings tm
		JOIN printer_configs pc ON pc.name = tm.printer_name
	`); err != nil {
		return err
	}

	var orphans int
	if err := tx.QueryRow(`
		SELECT COUNT(*) FROM toolhead_mappings tm
		LEFT JOIN printer_configs pc ON pc.name = tm.printer_name
		WHERE pc.printer_id IS NULL
	`).Scan(&orphans); err != nil {
		return err
	}
	if orphans > 0 {
		log.Printf("Migration: dropped %d toolhead mapping(s) without a matching printer", orphans)
	}

	if _, err := tx.Exec("DROP TABLE toolhead_mappings"); err != nil {
		return err
	}
	if _, err := tx.Exec("ALTER TABLE toolhead_mappings_v2 RENAME TO toolhead_mappings"); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	log.Printf("Migration: toolhead_mappings rebuilt with printer_id keys")
	return nil
}

// migrateToolheadNamesV2 merges toolhead_names into toolhead_mappings.display_name.
func (b *FilamentBridge) migrateToolheadNamesV2() error {
	exists, err := b.tableExists("toolhead_names")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	tx, err := b.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		INSERT INTO toolhead_mappings (printer_id, toolhead_id, display_name)
		SELECT printer_id, toolhead_id, display_name FROM toolhead_names WHERE display_name != ''
		ON CONFLICT(printer_id, toolhead_id) DO UPDATE SET display_name = excluded.display_name
	`); err != nil {
		return err
	}
	if _, err := tx.Exec("DROP TABLE toolhead_names"); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	log.Printf("Migration: toolhead_names merged into toolhead_mappings")
	return nil
}

// migratePrintHistoryV2 converts legacy print_history rows into print_jobs +
// filament_usage. Rows from the same printer/job finishing close together are
// grouped into a single job (the old table had one row per toolhead).
func (b *FilamentBridge) migratePrintHistoryV2() error {
	exists, err := b.tableExists("print_history")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	type historyRow struct {
		printerID    string
		toolheadID   int
		spoolID      int
		filamentUsed float64
		started      sql.NullTime
		finished     sql.NullTime
		jobName      string
	}

	rows, err := b.DB.Query(`
		SELECT pc.printer_id, ph.toolhead_id, ph.spool_id, ph.filament_used,
		       ph.print_started, ph.print_finished, COALESCE(ph.job_name, '')
		FROM print_history ph
		JOIN printer_configs pc ON pc.name = ph.printer_name
		ORDER BY ph.print_finished, ph.id
	`)
	if err != nil {
		return err
	}

	var entries []historyRow
	for rows.Next() {
		var r historyRow
		if err := rows.Scan(&r.printerID, &r.toolheadID, &r.spoolID, &r.filamentUsed, &r.started, &r.finished, &r.jobName); err != nil {
			rows.Close()
			return err
		}
		entries = append(entries, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	tx, err := b.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Group consecutive rows of the same printer/job within a 2-minute window
	// into a single job.
	const groupWindow = 2 * time.Minute
	var currentJobID int64
	var currentPrinter, currentJob string
	var currentFinished time.Time
	haveGroup := false

	for _, r := range entries {
		finished := time.Now()
		if r.finished.Valid {
			finished = r.finished.Time
		}

		sameGroup := haveGroup &&
			r.printerID == currentPrinter &&
			r.jobName == currentJob &&
			finished.Sub(currentFinished) <= groupWindow &&
			finished.Sub(currentFinished) >= -groupWindow

		if !sameGroup {
			var startedAt interface{}
			if r.started.Valid {
				startedAt = r.started.Time
			}
			res, err := tx.Exec(
				"INSERT INTO print_jobs (printer_id, job_name, started_at, finished_at, status) VALUES (?, ?, ?, ?, ?)",
				r.printerID, r.jobName, startedAt, finished, JobStatusCompleted,
			)
			if err != nil {
				return err
			}
			currentJobID, err = res.LastInsertId()
			if err != nil {
				return err
			}
			currentPrinter = r.printerID
			currentJob = r.jobName
			currentFinished = finished
			haveGroup = true
		}

		if _, err := tx.Exec(
			"INSERT INTO filament_usage (job_id, printer_id, toolhead_id, spool_id, grams, recorded_at) VALUES (?, ?, ?, ?, ?, ?)",
			currentJobID, r.printerID, r.toolheadID, r.spoolID, r.filamentUsed, finished,
		); err != nil {
			return err
		}
	}

	var orphans int
	if err := tx.QueryRow(`
		SELECT COUNT(*) FROM print_history ph
		LEFT JOIN printer_configs pc ON pc.name = ph.printer_name
		WHERE pc.printer_id IS NULL
	`).Scan(&orphans); err != nil {
		return err
	}
	if orphans > 0 {
		log.Printf("Migration: dropped %d print_history row(s) without a matching printer", orphans)
	}

	if _, err := tx.Exec("DROP TABLE print_history"); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	log.Printf("Migration: print_history converted into print_jobs + filament_usage (%d row(s))", len(entries))
	return nil
}

// migrateProcessedJobsV2 keeps the dedup state of processed_jobs by creating a
// closed print_jobs row for any job not already represented.
func (b *FilamentBridge) migrateProcessedJobsV2() error {
	exists, err := b.tableExists("processed_jobs")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	tx, err := b.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		INSERT INTO print_jobs (printer_id, job_name, finished_at, status)
		SELECT pj.printer_id, pj.filename, pj.processed_at, ?
		FROM processed_jobs pj
		WHERE NOT EXISTS (
			SELECT 1 FROM print_jobs j
			WHERE j.printer_id = pj.printer_id AND j.job_name = pj.filename
		)
	`, JobStatusCompleted); err != nil {
		return err
	}
	if _, err := tx.Exec("DROP TABLE processed_jobs"); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	log.Printf("Migration: processed_jobs converted into closed print_jobs")
	return nil
}
