package server

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

var devDbTableNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

var devDbSensitiveColumns = map[string]struct{}{
	"api_key":           {},
	"password":          {},
	"token":             {},
	"ha_token":          {},
	"spoolman_password": {},
}

const (
	devDbDefaultLimit = 100
	devDbMaxLimit     = 500
)

func isDevDbSensitiveConfigKey(key string) bool {
	lower := strings.ToLower(key)
	return strings.Contains(lower, "password") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "api_key")
}

func maskDevDbCell(columnName string, value interface{}) interface{} {
	if value == nil {
		return nil
	}

	str, ok := value.(string)
	if !ok {
		switch v := value.(type) {
		case []byte:
			str = string(v)
		default:
			return value
		}
	}

	if str == "" {
		return str
	}

	colLower := strings.ToLower(columnName)
	if _, sensitive := devDbSensitiveColumns[colLower]; sensitive {
		return "***"
	}

	return value
}

func maskDevDbRow(tableName string, columns []string, values []interface{}) map[string]interface{} {
	row := make(map[string]interface{}, len(columns))
	for i, col := range columns {
		val := values[i]
		colLower := strings.ToLower(col)

		if tableName == "configuration" && colLower == "value" {
			if keyIdx := indexOfColumn(columns, "key"); keyIdx >= 0 {
				if keyStr, ok := values[keyIdx].(string); ok && isDevDbSensitiveConfigKey(keyStr) {
					if val != nil {
						if s, ok := val.(string); ok && s != "" {
							row[col] = "***"
							continue
						}
					}
				}
			}
		}

		row[col] = maskDevDbCell(col, val)
	}
	return row
}

func indexOfColumn(columns []string, name string) int {
	for i, col := range columns {
		if strings.EqualFold(col, name) {
			return i
		}
	}
	return -1
}

func (ws *WebServer) devDbTableExists(name string) (bool, error) {
	var count int
	err := ws.bridge.DB.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'table' AND name = ? AND name NOT LIKE 'sqlite_%'
	`, name).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (ws *WebServer) devDbTableSchema(tableName string) ([]gin.H, error) {
	rows, err := ws.bridge.DB.Query(fmt.Sprintf("PRAGMA table_info(%q)", tableName))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schema []gin.H
	for rows.Next() {
		var cid, notNull, pk int
		var name, colType string
		var dfltValue interface{}
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return nil, err
		}

		col := gin.H{
			"name":        name,
			"type":        colType,
			"not_null":    notNull == 1,
			"primary_key": pk == 1,
		}
		if dfltValue != nil {
			switch v := dfltValue.(type) {
			case []byte:
				col["default_value"] = string(v)
			default:
				col["default_value"] = fmt.Sprint(v)
			}
		}
		schema = append(schema, col)
	}
	return schema, rows.Err()
}

func (ws *WebServer) devDbListTables() ([]gin.H, error) {
	rows, err := ws.bridge.DB.Query(`
		SELECT name FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []gin.H
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}

		var rowCount int
		countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM "%s"`, strings.ReplaceAll(name, `"`, `""`))
		if err := ws.bridge.DB.QueryRow(countQuery).Scan(&rowCount); err != nil {
			return nil, err
		}

		tables = append(tables, gin.H{
			"name":      name,
			"row_count": rowCount,
		})
	}

	return tables, rows.Err()
}

// devDbTablesHandler lists all user tables with row counts.
func (ws *WebServer) devDbTablesHandler(c *gin.Context) {
	tables, err := ws.devDbListTables()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tables": tables})
}

// devDbTableDataHandler returns paginated read-only rows for a table.
func (ws *WebServer) devDbTableDataHandler(c *gin.Context) {
	tableName := c.Param("name")
	if !devDbTableNameRegex.MatchString(tableName) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid table name"})
		return
	}

	exists, err := ws.devDbTableExists(tableName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Table not found"})
		return
	}

	limit := devDbDefaultLimit
	offset := 0

	if limitStr := c.Query("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > devDbMaxLimit {
		limit = devDbMaxLimit
	}

	if offsetStr := c.Query("offset"); offsetStr != "" {
		if parsed, err := strconv.Atoi(offsetStr); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	escapedName := strings.ReplaceAll(tableName, `"`, `""`)

	var total int
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM "%s"`, escapedName)
	if err := ws.bridge.DB.QueryRow(countQuery).Scan(&total); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	dataQuery := fmt.Sprintf(`SELECT * FROM "%s" LIMIT ? OFFSET ?`, escapedName)
	rows, err := ws.bridge.DB.Query(dataQuery, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resultRows := make([]map[string]interface{}, 0)
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		for i, val := range values {
			if b, ok := val.([]byte); ok {
				values[i] = string(b)
			}
		}

		resultRows = append(resultRows, maskDevDbRow(tableName, columns, values))
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	schema, err := ws.devDbTableSchema(tableName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"table":   tableName,
		"schema":  schema,
		"columns": columns,
		"rows":    resultRows,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}
