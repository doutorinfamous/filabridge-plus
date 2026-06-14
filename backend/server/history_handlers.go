package server

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

const (
	historyDefaultLimit = 25
	historyMaxLimit     = 200
)

// historyJobsHandler returns paginated print jobs with aggregated filament usage.
func (ws *WebServer) historyJobsHandler(c *gin.Context) {
	limit := historyDefaultLimit
	offset := 0

	if limitStr := c.Query("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > historyMaxLimit {
		limit = historyMaxLimit
	}
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if parsed, err := strconv.Atoi(offsetStr); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	printerID := c.Query("printer_id")

	jobs, total, err := ws.bridge.GetPrintJobs(printerID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"jobs":   jobs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// historyJobHandler returns a single print job with aggregated filament usage.
func (ws *WebServer) historyJobHandler(c *gin.Context) {
	jobID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid job ID"})
		return
	}

	job, err := ws.bridge.GetPrintJob(jobID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if job == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}

	c.JSON(http.StatusOK, job)
}
