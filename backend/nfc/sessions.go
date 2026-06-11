// Package nfc implements NFC/QR scan sessions: two-step pairing of a spool tag
// with a location tag, persisted in SQLite with expiration.
package nfc

import (
	"crypto/md5"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"filabridge/core"
)

// Session represents an active NFC scanning session.
type Session struct {
	SessionID         string    `json:"session_id"`
	SpoolID           int       `json:"spool_id"`
	PrinterName       string    `json:"printer_name"`
	ToolheadID        int       `json:"toolhead_id"`
	LocationName      string    `json:"location_name"`
	IsPrinterLocation bool      `json:"is_printer_location"`
	CreatedAt         time.Time `json:"created_at"`
	ExpiresAt         time.Time `json:"expires_at"`
	HasSpool          bool      `json:"has_spool"`
	HasLocation       bool      `json:"has_location"`
}

// hasLocationSet reports whether enough location fields are present for a session step.
// Moonraker toolheads set printerName; Bambu AMS slots set locationName only.
func hasLocationSet(isPrinterLocation bool, printerName string, toolheadID int, locationName string) bool {
	if locationName == "" {
		return false
	}
	if !isPrinterLocation {
		return true
	}
	if printerName != "" {
		return toolheadID >= 0
	}
	return true
}

// ParseLocationParam extracts location information from a location parameter.
// Supports multiple formats:
// 1. "PrinterName - Toolhead N" - printer toolhead locations (numeric ID)
// 2. "PrinterName - CustomName" - printer toolhead locations (custom name)
// 3. "LocationName" - non-printer locations (drybox, storage, etc.)
func ParseLocationParam(b *core.FilamentBridge, location string) (printerName string, toolheadID int, locationName string, isPrinterLocation bool, err error) {
	// Check if it contains " - " which indicates a printer toolhead location
	if strings.Contains(location, " - ") {
		parts := strings.SplitN(location, " - ", 2)
		if len(parts) == 2 {
			printerName = strings.TrimSpace(parts[0])
			toolheadPart := strings.TrimSpace(parts[1])

			// First, try to find by custom name (prioritize custom names over numeric parsing)
			printerConfigs, err := b.GetAllPrinterConfigs()
			if err == nil {
				for printerID, printerConfig := range printerConfigs {
					if printerConfig.Name == printerName {
						toolheadNames, err := b.GetAllToolheadNames(printerID)
						if err == nil {
							for tid, displayName := range toolheadNames {
								if displayName == toolheadPart {
									return printerName, tid, location, true, nil
								}
							}
						}
						// Also check default names
						for tid := 0; tid < printerConfig.Toolheads; tid++ {
							defaultName := core.DefaultToolheadDisplayName(tid)
							if defaultName == toolheadPart {
								return printerName, tid, location, true, nil
							}
						}
					}
				}
			}

			// Parse user-facing toolhead number (1-based): "Toolhead N" -> toolhead_id N-1
			if strings.HasPrefix(toolheadPart, "Toolhead ") {
				toolheadNumStr := strings.TrimPrefix(toolheadPart, "Toolhead ")
				toolheadNum, err := strconv.Atoi(toolheadNumStr)
				if err == nil && toolheadNum >= 1 {
					toolheadID := toolheadNum - 1
					printerConfigs, err := b.GetAllPrinterConfigs()
					if err == nil {
						for _, printerConfig := range printerConfigs {
							if printerConfig.Name == printerName {
								if toolheadID >= 0 && toolheadID < printerConfig.Toolheads {
									return printerName, toolheadID, location, true, nil
								}
								break
							}
						}
					}
				}
			}
			// If we couldn't parse it as a toolhead location, treat as regular location
		}
	}

	// For all other cases, treat as a location name
	return "", 0, location, false, nil
}

// SessionIDForIP creates a unique session ID based on client IP only.
// This ensures all scans from the same device use the same session.
func SessionIDForIP(clientIP string) string {
	hash := md5.Sum([]byte(clientIP))
	return fmt.Sprintf("%x", hash)[:16] // Use first 16 characters of MD5
}

// ClientIP extracts the real client IP from the request remote address.
func ClientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// If SplitHostPort fails, assume the whole string is the IP
		return remoteAddr
	}
	return host
}

// CreateOrUpdateSession creates a new session or updates an existing one.
func CreateOrUpdateSession(b *core.FilamentBridge, sessionID string, spoolID int, printerName string, toolheadID int, locationName string, isPrinterLocation bool) (*Session, error) {
	b.Mutex.Lock()
	defer b.Mutex.Unlock()

	var existingSession Session
	err := b.DB.QueryRow(
		"SELECT session_id, spool_id, printer_name, toolhead_id, location_name, is_printer_location, created_at, expires_at FROM nfc_sessions WHERE session_id = ?",
		sessionID,
	).Scan(&existingSession.SessionID, &existingSession.SpoolID, &existingSession.PrinterName,
		&existingSession.ToolheadID, &existingSession.LocationName, &existingSession.IsPrinterLocation, &existingSession.CreatedAt, &existingSession.ExpiresAt)

	if err == nil {
		now := time.Now()
		if now.After(existingSession.ExpiresAt) {
			// Session expired, create new one
			return createNewSession(b, sessionID, spoolID, printerName, toolheadID, locationName, isPrinterLocation)
		}

		// Update existing session - only update fields that are actually being set
		// This prevents overwriting existing data when scanning tags in sequence

		if spoolID > 0 {
			existingSession.SpoolID = spoolID
			existingSession.HasSpool = true

			_, err = b.DB.Exec(
				"UPDATE nfc_sessions SET spool_id = ? WHERE session_id = ?",
				spoolID, sessionID,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to update spool in NFC session: %w", err)
			}
		}

		if hasLocationSet(isPrinterLocation, printerName, toolheadID, locationName) {
			existingSession.PrinterName = printerName
			existingSession.ToolheadID = toolheadID
			existingSession.LocationName = locationName
			existingSession.IsPrinterLocation = isPrinterLocation
			existingSession.HasLocation = true

			_, err = b.DB.Exec(
				"UPDATE nfc_sessions SET printer_name = ?, toolhead_id = ?, location_name = ?, is_printer_location = ? WHERE session_id = ?",
				printerName, toolheadID, locationName, isPrinterLocation, sessionID,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to update location in NFC session: %w", err)
			}
		}

		// Recalculate flags based on current session data
		existingSession.HasSpool = existingSession.SpoolID > 0
		existingSession.HasLocation = hasLocationSet(
			existingSession.IsPrinterLocation,
			existingSession.PrinterName,
			existingSession.ToolheadID,
			existingSession.LocationName,
		)

		return &existingSession, nil
	}

	return createNewSession(b, sessionID, spoolID, printerName, toolheadID, locationName, isPrinterLocation)
}

// createNewSession creates a new NFC session.
func createNewSession(b *core.FilamentBridge, sessionID string, spoolID int, printerName string, toolheadID int, locationName string, isPrinterLocation bool) (*Session, error) {
	now := time.Now()
	expiresAt := now.Add(5 * time.Minute) // 5 minute expiration

	session := &Session{
		SessionID:         sessionID,
		SpoolID:           spoolID,
		PrinterName:       printerName,
		ToolheadID:        toolheadID,
		LocationName:      locationName,
		IsPrinterLocation: isPrinterLocation,
		CreatedAt:         now,
		ExpiresAt:         expiresAt,
		HasSpool:    spoolID > 0,
		HasLocation: hasLocationSet(isPrinterLocation, printerName, toolheadID, locationName),
	}

	_, err := b.DB.Exec(
		"INSERT INTO nfc_sessions (session_id, spool_id, printer_name, toolhead_id, location_name, is_printer_location, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		session.SessionID, session.SpoolID, session.PrinterName, session.ToolheadID, session.LocationName, session.IsPrinterLocation, session.CreatedAt, session.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create NFC session: %w", err)
	}

	return session, nil
}

// GetSession retrieves an existing NFC session.
func GetSession(b *core.FilamentBridge, sessionID string) (*Session, error) {
	var session Session
	err := b.DB.QueryRow(
		"SELECT session_id, spool_id, printer_name, toolhead_id, location_name, is_printer_location, created_at, expires_at FROM nfc_sessions WHERE session_id = ?",
		sessionID,
	).Scan(&session.SessionID, &session.SpoolID, &session.PrinterName,
		&session.ToolheadID, &session.LocationName, &session.IsPrinterLocation, &session.CreatedAt, &session.ExpiresAt)

	if err != nil {
		return nil, err
	}

	if time.Now().After(session.ExpiresAt) {
		DeleteSession(b, sessionID)
		return nil, fmt.Errorf("session expired")
	}

	session.HasSpool = session.SpoolID > 0
	session.HasLocation = hasLocationSet(
		session.IsPrinterLocation,
		session.PrinterName,
		session.ToolheadID,
		session.LocationName,
	)

	return &session, nil
}

// IsComplete checks if both spool and location are set.
func (s *Session) IsComplete() bool {
	return s.HasSpool && s.HasLocation
}

// DeleteSession removes a session from the database.
func DeleteSession(b *core.FilamentBridge, sessionID string) error {
	_, err := b.DB.Exec("DELETE FROM nfc_sessions WHERE session_id = ?", sessionID)
	return err
}

// CleanupExpiredSessions removes sessions older than their expiration time.
func CleanupExpiredSessions(b *core.FilamentBridge) error {
	now := time.Now()
	_, err := b.DB.Exec("DELETE FROM nfc_sessions WHERE expires_at < ?", now)
	if err != nil {
		log.Printf("Error cleaning up expired NFC sessions: %v", err)
		return err
	}
	return nil
}
