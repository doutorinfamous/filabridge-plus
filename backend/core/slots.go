package core

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"filabridge/spoolman"
)

// PrinterSlot is a single assignable filament position on a printer: a
// Moonraker toolhead, a Bambu AMS tray or an external spool holder. All slot
// types live in the unified printer_slots table, keyed by slot_id
// ("<printer_id>:T<n>" for toolheads, the HA tray unique_id for Bambu trays).
type PrinterSlot struct {
	SlotID      string     `json:"slot_id"`
	PrinterID   string     `json:"printer_id"`
	SlotType    string     `json:"slot_type"`
	ToolheadID  *int       `json:"toolhead_id,omitempty"`
	EntityID    string     `json:"entity_id,omitempty"`
	AMSNumber   int        `json:"ams_number,omitempty"`
	TrayNumber  int        `json:"tray_number,omitempty"`
	DisplayName string     `json:"display_name,omitempty"`
	SpoolID     int        `json:"spool_id"`
	MappedAt    *time.Time `json:"mapped_at,omitempty"`
}

// IsTray reports whether the slot is a Bambu AMS tray or external spool holder.
func (s PrinterSlot) IsTray() bool {
	return s.SlotType == SlotTypeAMSTray || s.SlotType == SlotTypeExternal
}

// ToolheadSlotID builds the printer_slots key for a Moonraker toolhead.
func ToolheadSlotID(printerID string, toolheadID int) string {
	return fmt.Sprintf("%s:T%d", printerID, toolheadID)
}

const slotSelectColumns = `slot_id, printer_id, slot_type, toolhead_id, COALESCE(entity_id, ''),
	COALESCE(ams_number, 0), COALESCE(tray_number, 0), display_name, COALESCE(spool_id, 0), mapped_at`

type slotRowScanner interface {
	Scan(dest ...interface{}) error
}

func scanSlot(row slotRowScanner) (PrinterSlot, error) {
	var slot PrinterSlot
	var toolheadID sql.NullInt64
	var mappedAt sql.NullTime
	err := row.Scan(
		&slot.SlotID, &slot.PrinterID, &slot.SlotType, &toolheadID, &slot.EntityID,
		&slot.AMSNumber, &slot.TrayNumber, &slot.DisplayName, &slot.SpoolID, &mappedAt,
	)
	if err != nil {
		return slot, err
	}
	if toolheadID.Valid {
		id := int(toolheadID.Int64)
		slot.ToolheadID = &id
	}
	if mappedAt.Valid {
		t := mappedAt.Time
		slot.MappedAt = &t
	}
	return slot, nil
}

// GetSlot returns a slot by its slot_id, or nil when not found.
func (b *FilamentBridge) GetSlot(slotID string) (*PrinterSlot, error) {
	row := b.DB.QueryRow(
		fmt.Sprintf("SELECT %s FROM printer_slots WHERE slot_id = ?", slotSelectColumns),
		slotID,
	)
	slot, err := scanSlot(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get slot %s: %w", slotID, err)
	}
	return &slot, nil
}

// GetSlotsForPrinter returns all slots for a printer ordered by type and position.
func (b *FilamentBridge) GetSlotsForPrinter(printerID string) ([]PrinterSlot, error) {
	rows, err := b.DB.Query(
		fmt.Sprintf(`SELECT %s FROM printer_slots WHERE printer_id = ?
			ORDER BY slot_type, toolhead_id, ams_number, tray_number`, slotSelectColumns),
		printerID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get slots for printer %s: %w", printerID, err)
	}
	defer rows.Close()

	var slots []PrinterSlot
	for rows.Next() {
		slot, err := scanSlot(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan slot row: %w", err)
		}
		slots = append(slots, slot)
	}
	return slots, rows.Err()
}

// SetSlotSpool assigns a spool to a slot, creating a minimal row when the slot
// is not known yet (e.g. a Bambu tray seen before SyncTrays ran).
func (b *FilamentBridge) SetSlotSpool(slotID, printerID, slotType, displayName string, spoolID int) error {
	_, err := b.DB.Exec(`
		INSERT INTO printer_slots (slot_id, printer_id, slot_type, display_name, spool_id, mapped_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(slot_id) DO UPDATE SET spool_id = excluded.spool_id, mapped_at = excluded.mapped_at
	`, slotID, printerID, slotType, displayName, spoolID, time.Now())
	if err != nil {
		return fmt.Errorf("failed to set slot spool: %w", err)
	}
	return nil
}

// ClearSlotSpool removes the spool from a slot (the slot row is kept).
func (b *FilamentBridge) ClearSlotSpool(slotID string) error {
	_, err := b.DB.Exec(
		"UPDATE printer_slots SET spool_id = NULL, mapped_at = ? WHERE slot_id = ?",
		time.Now(), slotID,
	)
	if err != nil {
		return fmt.Errorf("failed to clear slot spool: %w", err)
	}
	return nil
}

// FindSlotsBySpool returns all slots currently holding the given spool.
func (b *FilamentBridge) FindSlotsBySpool(spoolID int) ([]PrinterSlot, error) {
	rows, err := b.DB.Query(
		fmt.Sprintf("SELECT %s FROM printer_slots WHERE spool_id = ?", slotSelectColumns),
		spoolID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find slots by spool: %w", err)
	}
	defer rows.Close()

	var slots []PrinterSlot
	for rows.Next() {
		slot, err := scanSlot(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan slot row: %w", err)
		}
		slots = append(slots, slot)
	}
	return slots, rows.Err()
}

// ClearSpoolFromSlots removes a spool from every slot except keepSlotID
// (empty keeps nothing). Returns the slots that were cleared so callers can
// mirror the change to Spoolman.
func (b *FilamentBridge) ClearSpoolFromSlots(spoolID int, keepSlotID string) ([]PrinterSlot, error) {
	slots, err := b.FindSlotsBySpool(spoolID)
	if err != nil {
		return nil, err
	}

	var cleared []PrinterSlot
	for _, slot := range slots {
		if keepSlotID != "" && slot.SlotID == keepSlotID {
			continue
		}
		if err := b.ClearSlotSpool(slot.SlotID); err != nil {
			return cleared, err
		}
		cleared = append(cleared, slot)
	}
	return cleared, nil
}

// BackfillTraySpoolAssignments fills printer_slots.spool_id for Bambu trays
// from Spoolman extra.active_tray after the v3 migration. It runs once; when
// Spoolman is unreachable it is retried on the next startup.
func (b *FilamentBridge) BackfillTraySpoolAssignments() error {
	done, err := b.GetConfigValue(ConfigKeySlotsTrayBackfillDone)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if done != "false" {
		return nil // Never flagged as pending, or already done
	}

	spools, err := b.Spoolman.GetAllSpools()
	if err != nil {
		return fmt.Errorf("spoolman unreachable, tray spool backfill postponed: %w", err)
	}

	filled := 0
	for i := range spools {
		trayID := spoolman.GetSpoolExtraString(&spools[i], spoolman.ExtraFieldActiveTray)
		if trayID == "" {
			continue
		}
		res, err := b.DB.Exec(`
			UPDATE printer_slots SET spool_id = ?
			WHERE slot_type IN (?, ?) AND (slot_id = ? OR entity_id = ?)
		`, spools[i].ID, SlotTypeAMSTray, SlotTypeExternal, trayID, trayID)
		if err != nil {
			return fmt.Errorf("failed to backfill tray %s: %w", trayID, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			filled++
		}
	}

	if err := b.SetConfigValue(ConfigKeySlotsTrayBackfillDone, "true"); err != nil {
		return err
	}
	log.Printf("Migration: backfilled %d tray spool assignment(s) from Spoolman into printer_slots", filled)
	return nil
}
