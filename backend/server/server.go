// Package server exposes the FilaBridge HTTP API (JSON) and the status WebSocket.
// The UI itself is served by the Next.js app in /web, which proxies /api and /ws here.
package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"filabridge/core"
	"filabridge/spoolman"
)

// WebServer handles HTTP requests using Gin.
type WebServer struct {
	bridge         *core.FilamentBridge
	router         *gin.Engine
	operationMutex sync.Mutex // Protects add/update/delete printer operations
	wsHub          *WebSocketHub
}

// WebSocketHub manages WebSocket connections and broadcasts.
type WebSocketHub struct {
	clients    map[*WebSocketClient]bool
	register   chan *WebSocketClient
	unregister chan *WebSocketClient
	broadcast  chan []byte
	mutex      sync.RWMutex
}

// WebSocketClient represents a WebSocket connection.
type WebSocketClient struct {
	hub  *WebSocketHub
	conn *websocket.Conn
	send chan []byte
}

// WebSocketMessage represents the structure of messages sent to clients.
type WebSocketMessage struct {
	Type             string                                  `json:"type"`
	Timestamp        time.Time                               `json:"timestamp"`
	Printers         map[string]core.PrinterData             `json:"printers"`
	Spools           []spoolman.Spool                        `json:"spools"`
	ToolheadMappings map[string]map[int]core.ToolheadMapping `json:"toolhead_mappings"`
	PrintErrors      []core.PrintError                       `json:"print_errors,omitempty"`
}

// NewWebServer creates a new web server with Gin.
func NewWebServer(bridge *core.FilamentBridge) *WebServer {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// Custom recovery middleware for API routes to ensure JSON responses
	router.Use(func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				if strings.HasPrefix(c.Request.URL.Path, "/api/") {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
					c.Abort()
				} else {
					c.AbortWithStatus(http.StatusInternalServerError)
				}
			}
		}()
		c.Next()
	})

	wsHub := &WebSocketHub{
		clients:    make(map[*WebSocketClient]bool),
		register:   make(chan *WebSocketClient),
		unregister: make(chan *WebSocketClient),
		broadcast:  make(chan []byte),
	}

	ws := &WebServer{
		bridge: bridge,
		router: router,
		wsHub:  wsHub,
	}

	go wsHub.run()

	ws.setupRoutes()
	return ws
}

// setupRoutes configures all the routes. Endpoint paths are unchanged from the
// previous monolith — the Next.js proxy forwards them verbatim.
func (ws *WebServer) setupRoutes() {
	api := ws.router.Group("/api")
	{
		api.GET("/status", ws.statusHandler)
		api.GET("/spools", ws.spoolsHandler)
		api.GET("/filaments", ws.filamentsHandler)
		api.POST("/map_toolhead", ws.mapToolheadHandler)
		api.GET("/available_spools", ws.availableSpoolsHandler)
		api.GET("/spoolman/test", ws.testSpoolmanConnectionHandler)
		api.GET("/spoolman/debug", ws.debugSpoolmanHandler)
		api.POST("/test/print_complete", ws.testPrintCompleteHandler)
		api.GET("/config", ws.getConfigHandler)
		api.POST("/config", ws.updateConfigHandler)
		api.GET("/config/auto-assign-previous-spool", ws.getAutoAssignPreviousSpoolHandler)
		api.PUT("/config/auto-assign-previous-spool", ws.updateAutoAssignPreviousSpoolHandler)
		api.GET("/printers", ws.getPrintersHandler)
		api.POST("/printers", ws.addPrinterHandler)
		api.PUT("/printers/:id", ws.updatePrinterHandler)
		api.DELETE("/printers/:id", ws.deletePrinterHandler)
		api.GET("/printers/:id/toolheads", ws.getToolheadNamesHandler)
		api.PUT("/printers/:id/toolheads/:toolhead_id", ws.updateToolheadNameHandler)
		api.POST("/detect_printer", ws.detectPrinterHandler)
		api.GET("/print-errors", ws.getPrintErrorsHandler)
		api.POST("/print-errors/:id/acknowledge", ws.acknowledgePrintErrorHandler)
		api.GET("/history/jobs", ws.historyJobsHandler)
		api.GET("/history/jobs/:id", ws.historyJobHandler)
		api.GET("/nfc/assign", ws.nfcAssignHandler)
		api.GET("/nfc/urls", ws.nfcUrlsHandler)
		api.GET("/nfc/session/status", ws.nfcSessionStatusHandler)
		api.GET("/dev/db/tables", ws.devDbTablesHandler)
		api.GET("/dev/db/tables/:name", ws.devDbTableDataHandler)
		api.GET("/locations", ws.getLocationsHandler)
		api.GET("/locations/:name/status", ws.getLocationStatusHandler)
		api.POST("/locations", ws.createLocationHandler)
		api.PUT("/locations/:name", ws.updateLocationHandler)
		api.DELETE("/locations/:name", ws.deleteLocationHandler)
		ws.registerBambuRoutes(api)
	}

	// WebSocket endpoint
	ws.router.GET("/ws/status", ws.websocketHandler)
}

// requestHost returns the externally visible host for building absolute URLs
// (NFC tags/QR codes). Behind the Next.js proxy the original host arrives in
// X-Forwarded-Host.
func requestHost(c *gin.Context) string {
	if fwd := c.GetHeader("X-Forwarded-Host"); fwd != "" {
		return fwd
	}
	return c.Request.Host
}

// nfcBaseURL returns the base URL used to build NFC/QR tag URLs. The
// configured filabridge_public_url takes priority (tags must be reachable
// from phones on the LAN); the request host is only a fallback.
func nfcBaseURL(bridge *core.FilamentBridge, c *gin.Context) string {
	if v, err := bridge.GetConfigValue(core.ConfigKeyFilabridgePublicURL); err == nil {
		if u := strings.TrimRight(strings.TrimSpace(v), "/"); u != "" {
			return u
		}
	}
	return "http://" + requestHost(c)
}

// WebSocket hub methods

// run starts the WebSocket hub.
func (h *WebSocketHub) run() {
	for {
		select {
		case client := <-h.register:
			h.mutex.Lock()
			h.clients[client] = true
			h.mutex.Unlock()
			log.Printf("WebSocket client connected. Total clients: %d", len(h.clients))

		case client := <-h.unregister:
			h.mutex.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mutex.Unlock()
			log.Printf("WebSocket client disconnected. Total clients: %d", len(h.clients))

		case message := <-h.broadcast:
			h.mutex.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mutex.RUnlock()
		}
	}
}

// BroadcastStatus sends status updates to all connected clients.
func (ws *WebServer) BroadcastStatus() {
	status, err := ws.BuildStatus()
	if err != nil {
		log.Printf("Error getting status for broadcast: %v", err)
		return
	}

	spools, err := ws.bridge.Spoolman.GetAllSpools()
	if err != nil {
		log.Printf("Error getting spools for broadcast: %v", err)
		spools = []spoolman.Spool{}
	}

	printErrors := ws.bridge.GetPrintErrors()

	message := WebSocketMessage{
		Type:             "status_update",
		Timestamp:        time.Now(),
		Printers:         status.Printers,
		Spools:           spools,
		ToolheadMappings: status.ToolheadMappings,
		PrintErrors:      printErrors,
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		log.Printf("Error marshaling WebSocket message: %v", err)
		return
	}

	select {
	case ws.wsHub.broadcast <- jsonData:
		log.Printf("Broadcasted status update to %d clients", len(ws.wsHub.clients))
	default:
		log.Printf("No clients connected to receive broadcast")
	}
}

// websocketHandler handles WebSocket connections.
func (ws *WebServer) websocketHandler(c *gin.Context) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow connections from any origin (behind trusted proxy)
		},
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := &WebSocketClient{
		hub:  ws.wsHub,
		conn: conn,
		send: make(chan []byte, 256),
	}

	client.hub.register <- client

	go client.writePump()
	go client.readPump()
}

// readPump pumps messages from the WebSocket connection to the hub.
func (c *WebSocketClient) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}
	}
}

// writePump pumps messages from the hub to the WebSocket connection.
func (c *WebSocketClient) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued messages to the current websocket message
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Start starts the web server.
func (ws *WebServer) Start(addr string) error {
	return ws.router.Run(addr)
}
