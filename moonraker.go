package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SnapmakerU1MoonrakerClient handles communication with Snapmaker U1 Moonraker API.
type SnapmakerU1MoonrakerClient struct {
	baseURL              string
	apiKey               string
	httpClient           *http.Client
	fileDownloadTimeout  int
}

// MoonrakerPrinterStatus represents normalized printer status for the bridge.
type MoonrakerPrinterStatus struct {
	State          string
	RawState       string
	JobFilename    string
	JobDisplayName string
	Progress       float64
	PrintDuration  float64
	FilamentUsed   float64 // extruded filament length in mm from print_stats
	CurrentLayer   *int
	TotalLayer     *int
}

// MoonrakerPrinterInfo represents printer identification data.
type MoonrakerPrinterInfo struct {
	Hostname        string
	SoftwareVersion string
	State           string
}

type moonrakerResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type moonrakerObjectsQueryResult struct {
	Status struct {
		PrintStats struct {
			State         string  `json:"state"`
			Filename      string  `json:"filename"`
			PrintDuration float64 `json:"print_duration"`
			FilamentUsed  float64 `json:"filament_used"`
			Info          struct {
				TotalLayer   *int `json:"total_layer"`
				CurrentLayer *int `json:"current_layer"`
			} `json:"info"`
		} `json:"print_stats"`
		VirtualSDCard struct {
			Progress float64 `json:"progress"`
		} `json:"virtual_sdcard"`
	} `json:"status"`
}

type moonrakerPrinterInfoResult struct {
	Hostname        string `json:"hostname"`
	SoftwareVersion string `json:"software_version"`
	State           string `json:"state"`
}

type moonrakerServerInfoResult struct {
	KlipperConnected bool   `json:"klipper_connected"`
	MoonrakerVersion string `json:"moonraker_version"`
}

type moonrakerFileMetadataResult struct {
	Filename            string    `json:"filename"`
	Size                int64     `json:"size"`
	EstimatedTime       float64   `json:"estimated_time"`
	FilamentTotal       float64   `json:"filament_total"`
	FilamentWeightTotal float64   `json:"filament_weight_total"`
	FilamentWeights     []float64 `json:"filament_weights"`
	FilamentWeight      []float64 `json:"filament_weight"` // SnapmakerOrca uses singular field name
	ReferencedTools     []int     `json:"referenced_tools"`
}

type moonrakerReprintInfo struct {
	ExtruderMapTable []int  `json:"extruder_map_table"`
	ExtrudersUsed    []bool `json:"extruders_used"`
}

type moonrakerPrintTaskConfigResult struct {
	ExtruderMapTable []int                `json:"extruder_map_table"`
	ExtrudersUsed    []bool               `json:"extruders_used"`
	ReprintInfo      moonrakerReprintInfo `json:"reprint_info"`
}

// SnapmakerExtruderMapping holds main and reprint extruder maps from print_task_config.
type SnapmakerExtruderMapping struct {
	MapTable             []int
	ReprintMapTable      []int
	ExtrudersUsed        []bool
	ReprintExtrudersUsed []bool
}

type moonrakerObjectsQueryPrintTaskResult struct {
	Status struct {
		PrintTaskConfig moonrakerPrintTaskConfigResult `json:"print_task_config"`
	} `json:"status"`
}

func moonrakerMetadataToFilamentUsage(m *moonrakerFileMetadataResult) *FilamentUsageMetadata {
	if m == nil {
		return nil
	}
	weights := m.FilamentWeights
	if len(weights) == 0 {
		weights = m.FilamentWeight
	}
	return &FilamentUsageMetadata{
		FilamentWeights:     weights,
		FilamentWeightTotal: m.FilamentWeightTotal,
		ReferencedTools:     m.ReferencedTools,
		Size:                m.Size,
	}
}

// NewSnapmakerU1MoonrakerClient creates a client for Snapmaker U1 Moonraker.
func NewSnapmakerU1MoonrakerClient(ipAddress, apiKey string, timeout, fileDownloadTimeout int) *SnapmakerU1MoonrakerClient {
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       30 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &SnapmakerU1MoonrakerClient{
		baseURL:             normalizeMoonrakerBaseURL(ipAddress),
		apiKey:              apiKey,
		fileDownloadTimeout: fileDownloadTimeout,
		httpClient: &http.Client{
			Timeout:   time.Duration(timeout) * time.Second,
			Transport: transport,
		},
	}
}

func normalizeMoonrakerBaseURL(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}
	if strings.HasPrefix(address, "http://") || strings.HasPrefix(address, "https://") {
		return strings.TrimRight(address, "/")
	}
	return "http://" + strings.TrimRight(address, "/")
}

func (c *SnapmakerU1MoonrakerClient) addAuth(req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("X-Api-Key", c.apiKey)
	}
}

func (c *SnapmakerU1MoonrakerClient) doRequest(method, path string) ([]byte, error) {
	req, err := http.NewRequest(method, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.addAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Moonraker API error: %d - %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// TestConnection verifies basic connectivity to Moonraker.
func (c *SnapmakerU1MoonrakerClient) TestConnection() error {
	_, err := c.GetServerInfo()
	return err
}

// GetServerInfo returns basic Moonraker server information.
func (c *SnapmakerU1MoonrakerClient) GetServerInfo() (*moonrakerServerInfoResult, error) {
	body, err := c.doRequest(http.MethodGet, "/server/info")
	if err != nil {
		return nil, err
	}

	var envelope moonrakerResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("failed to decode server info envelope: %w", err)
	}
	if envelope.Error != nil {
		return nil, fmt.Errorf("Moonraker API error: %s", envelope.Error.Message)
	}

	var info moonrakerServerInfoResult
	if err := json.Unmarshal(envelope.Result, &info); err != nil {
		return nil, fmt.Errorf("failed to decode server info: %w", err)
	}

	return &info, nil
}

// GetPrinterInfo returns printer identification details.
func (c *SnapmakerU1MoonrakerClient) GetPrinterInfo() (*MoonrakerPrinterInfo, error) {
	log.Printf("🔍 [Moonraker] Getting printer info from %s", c.baseURL)

	body, err := c.doRequest(http.MethodGet, "/printer/info")
	if err != nil {
		log.Printf("❌ [Moonraker] API call failed for %s: %v", c.baseURL, err)
		return nil, err
	}

	var envelope moonrakerResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("failed to decode printer info envelope: %w", err)
	}
	if envelope.Error != nil {
		return nil, fmt.Errorf("Moonraker API error: %s", envelope.Error.Message)
	}

	var result moonrakerPrinterInfoResult
	if err := json.Unmarshal(envelope.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to decode printer info: %w", err)
	}

	info := &MoonrakerPrinterInfo{
		Hostname:        result.Hostname,
		SoftwareVersion: result.SoftwareVersion,
		State:           result.State,
	}

	log.Printf("✅ [Moonraker] Parsed printer info from %s: hostname='%s', version='%s', state='%s'",
		c.baseURL, info.Hostname, info.SoftwareVersion, info.State)

	return info, nil
}

// GetPrinterStatus returns normalized printer and job status.
func (c *SnapmakerU1MoonrakerClient) GetPrinterStatus() (*MoonrakerPrinterStatus, error) {
	body, err := c.doRequest(http.MethodGet, "/printer/objects/query?print_stats&virtual_sdcard")
	if err != nil {
		return nil, fmt.Errorf("failed to get printer status from Moonraker: %w", err)
	}

	var envelope moonrakerResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("failed to decode status envelope: %w", err)
	}
	if envelope.Error != nil {
		return nil, fmt.Errorf("Moonraker API error: %s", envelope.Error.Message)
	}

	var result moonrakerObjectsQueryResult
	if err := json.Unmarshal(envelope.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to decode status response: %w", err)
	}

	rawState := strings.ToLower(strings.TrimSpace(result.Status.PrintStats.State))
	filename := strings.TrimSpace(result.Status.PrintStats.Filename)

	status := &MoonrakerPrinterStatus{
		State:          normalizeMoonrakerState(rawState),
		RawState:       rawState,
		JobFilename:    filename,
		JobDisplayName: displayNameFromFilename(filename),
		Progress:       result.Status.VirtualSDCard.Progress,
		PrintDuration:  result.Status.PrintStats.PrintDuration,
		FilamentUsed:   result.Status.PrintStats.FilamentUsed,
		CurrentLayer:   result.Status.PrintStats.Info.CurrentLayer,
		TotalLayer:     result.Status.PrintStats.Info.TotalLayer,
	}

	return status, nil
}

// computeTimeRemainingSeconds estimates remaining print time in seconds.
// Prefers slicer metadata; falls back to progress-based extrapolation.
func computeTimeRemainingSeconds(printDuration, progress, estimatedTime float64) *float64 {
	if estimatedTime > 0 {
		remaining := estimatedTime - printDuration
		if remaining < 0 {
			remaining = 0
		}
		return &remaining
	}
	if progress > 0.001 && printDuration > 0 {
		total := printDuration / progress
		remaining := total - printDuration
		if remaining < 0 {
			remaining = 0
		}
		return &remaining
	}
	return nil
}

func formatDurationSeconds(seconds float64) string {
	if seconds < 0 {
		return "--"
	}
	total := int(seconds + 0.5)
	if total <= 0 {
		return "0s"
	}
	hours := total / 3600
	minutes := (total % 3600) / 60
	secs := total % 60
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, secs)
	}
	return fmt.Sprintf("%ds", secs)
}

// filamentLengthMmToGrams converts extruded filament length (mm) to weight (grams).
func filamentLengthMmToGrams(lengthMm, diameterMm, densityGcm3 float64) float64 {
	if lengthMm <= 0 || diameterMm <= 0 || densityGcm3 <= 0 {
		return 0
	}
	radiusMm := diameterMm / 2
	volumeMm3 := math.Pi * radiusMm * radiusMm * lengthMm
	return volumeMm3 * densityGcm3 / 1000
}

// GetGcodeFile downloads a G-code file from Moonraker.
func (c *SnapmakerU1MoonrakerClient) GetGcodeFile(filename string) ([]byte, error) {
	path := "/server/files/gcodes/" + escapeMoonrakerFilePath(filename)
	body, err := c.doRequest(http.MethodGet, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get G-code file from Moonraker: %w", err)
	}
	return body, nil
}

// GetGcodeFileWithRetry downloads the G-code file with retry logic and exponential backoff.
func (c *SnapmakerU1MoonrakerClient) GetGcodeFileWithRetry(filename string, fileDownloadTimeout int) ([]byte, error) {
	const maxRetries = 3
	backoffDelays := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second}

	if fileDownloadTimeout <= 0 {
		fileDownloadTimeout = c.fileDownloadTimeout
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		log.Printf("Downloading G-code file attempt %d/%d: %s", attempt+1, maxRetries, filename)

		fileDialer := &net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}

		fileClient := &http.Client{
			Timeout: time.Duration(fileDownloadTimeout) * time.Second,
			Transport: &http.Transport{
				DialContext:           fileDialer.DialContext,
				MaxIdleConns:          10,
				MaxIdleConnsPerHost:   2,
				IdleConnTimeout:       90 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		}

		path := "/server/files/gcodes/" + escapeMoonrakerFilePath(filename)
		req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
		if err != nil {
			lastErr = fmt.Errorf("failed to create G-code request: %w", err)
			log.Printf("Attempt %d failed: %v", attempt+1, lastErr)
			if attempt < maxRetries-1 {
				time.Sleep(backoffDelays[attempt])
			}
			continue
		}

		c.addAuth(req)

		resp, err := fileClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("failed to get G-code file from Moonraker: %w", err)
			log.Printf("Attempt %d failed: %v", attempt+1, lastErr)
			if attempt < maxRetries-1 {
				time.Sleep(backoffDelays[attempt])
			}
			continue
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("Moonraker API error: %d - %s", resp.StatusCode, string(body))
			log.Printf("Attempt %d failed: %v", attempt+1, lastErr)
			if attempt < maxRetries-1 {
				time.Sleep(backoffDelays[attempt])
			}
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("failed to read G-code file: %w", err)
			log.Printf("Attempt %d failed: %v", attempt+1, lastErr)
			if attempt < maxRetries-1 {
				time.Sleep(backoffDelays[attempt])
			}
			continue
		}

		log.Printf("Successfully downloaded G-code file on attempt %d: %s (%d bytes)",
			attempt+1, filename, len(body))
		return body, nil
	}

	return nil, fmt.Errorf("failed to download G-code file after %d attempts: %w", maxRetries, lastErr)
}

// GetFileMetadata returns metadata for a G-code file when available.
func (c *SnapmakerU1MoonrakerClient) GetFileMetadata(filename string) (*moonrakerFileMetadataResult, error) {
	path := "/server/files/metadata?filename=" + url.QueryEscape(filename)
	body, err := c.doRequest(http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var envelope moonrakerResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("failed to decode metadata envelope: %w", err)
	}
	if envelope.Error != nil {
		return nil, fmt.Errorf("Moonraker API error: %s", envelope.Error.Message)
	}

	var metadata moonrakerFileMetadataResult
	if err := json.Unmarshal(envelope.Result, &metadata); err != nil {
		return nil, fmt.Errorf("failed to decode metadata response: %w", err)
	}

	return &metadata, nil
}

// GetPrintTaskFilamentMapping returns Snapmaker print_task_config mapping for the current/last print.
func (c *SnapmakerU1MoonrakerClient) GetPrintTaskFilamentMapping() SnapmakerExtruderMapping {
	body, err := c.doRequest(http.MethodGet, "/printer/objects/query?print_task_config")
	if err != nil {
		return SnapmakerExtruderMapping{}
	}

	var envelope moonrakerResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return SnapmakerExtruderMapping{}
	}
	if envelope.Error != nil {
		return SnapmakerExtruderMapping{}
	}

	var result moonrakerObjectsQueryPrintTaskResult
	if err := json.Unmarshal(envelope.Result, &result); err != nil {
		return SnapmakerExtruderMapping{}
	}

	cfg := result.Status.PrintTaskConfig
	return SnapmakerExtruderMapping{
		MapTable:             cfg.ExtruderMapTable,
		ReprintMapTable:      cfg.ReprintInfo.ExtruderMapTable,
		ExtrudersUsed:        cfg.ExtrudersUsed,
		ReprintExtrudersUsed: cfg.ReprintInfo.ExtrudersUsed,
	}
}

func applySnapmakerExtruderMapping(metadata *FilamentUsageMetadata, mapping SnapmakerExtruderMapping) {
	if metadata == nil {
		return
	}
	metadata.ExtruderMapTable = mapping.MapTable
	metadata.ReprintExtruderMapTable = mapping.ReprintMapTable
	metadata.ExtrudersUsed = mapping.ExtrudersUsed
	metadata.ReprintExtrudersUsed = mapping.ReprintExtrudersUsed
}

func countTrue(flags []bool) int {
	n := 0
	for _, v := range flags {
		if v {
			n++
		}
	}
	return n
}

// ParseFilamentUsageFromFile downloads the exact G-code file and resolves per-extruder usage.
func (c *SnapmakerU1MoonrakerClient) ParseFilamentUsageFromFile(filename string, fileDownloadTimeout int) (map[int]float64, error) {
	gcodeContent, err := c.GetGcodeFileWithRetry(filename, fileDownloadTimeout)
	if err != nil {
		return nil, err
	}

	var metadata *FilamentUsageMetadata
	if meta, metaErr := c.GetFileMetadata(filename); metaErr == nil {
		metadata = moonrakerMetadataToFilamentUsage(meta)
		if metadata != nil && metadata.Size > 0 && int64(len(gcodeContent)) != metadata.Size {
			log.Printf("Warning: G-code size mismatch for %s (downloaded %d bytes, metadata %d bytes)",
				filename, len(gcodeContent), metadata.Size)
		}
	}
	if metadata == nil {
		metadata = &FilamentUsageMetadata{}
	}
	applySnapmakerExtruderMapping(metadata, c.GetPrintTaskFilamentMapping())

	resolution := ResolveFilamentUsage(gcodeContent, metadata)
	logFilamentUsageResolution(filename, resolution.Source, resolution.Usage)
	return resolution.Usage, nil
}

func normalizeMoonrakerState(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case MoonrakerStatePrinting, MoonrakerStatePaused:
		return StatePrinting
	case MoonrakerStateComplete, MoonrakerStateCancelled:
		return StateFinished
	case MoonrakerStateStandby:
		return StateIdle
	case MoonrakerStateError:
		return StateError
	default:
		return StateIdle
	}
}

func isMoonrakerPrintingState(rawState string) bool {
	switch strings.ToLower(strings.TrimSpace(rawState)) {
	case MoonrakerStatePrinting, MoonrakerStatePaused:
		return true
	default:
		return false
	}
}

func isMoonrakerFinishedState(rawState string) bool {
	switch strings.ToLower(strings.TrimSpace(rawState)) {
	case MoonrakerStateComplete, MoonrakerStateStandby:
		return true
	default:
		return false
	}
}

func isMoonrakerCancelledState(rawState string) bool {
	return strings.ToLower(strings.TrimSpace(rawState)) == MoonrakerStateCancelled
}

func displayNameFromFilename(filename string) string {
	if filename == "" {
		return "No active job"
	}
	parts := strings.Split(filename, "/")
	return parts[len(parts)-1]
}

func escapeMoonrakerFilePath(filename string) string {
	filename = strings.TrimPrefix(filename, "/")
	segments := strings.Split(filename, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}
