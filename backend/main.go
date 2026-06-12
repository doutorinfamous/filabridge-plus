package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"filabridge/core"
	"filabridge/nfc"
	"filabridge/server"
	"filabridge/snapmaker"
)

func main() {
	// Command line flags
	var (
		webOnly    = flag.Bool("web-only", false, "Run only the web interface")
		bridgeOnly = flag.Bool("bridge-only", false, "Run only the bridge service")
		port       = flag.String("port", core.DefaultWebPort, "API port")
		host       = flag.String("host", "0.0.0.0", "API host")
	)
	flag.Parse()

	portFlagSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "port" {
			portFlagSet = true
		}
	})

	// Create bridge instance first (with default config)
	bridge, err := core.NewFilamentBridge(nil)
	if err != nil {
		log.Fatalf("Failed to create bridge: %v", err)
	}
	defer bridge.Close()

	// Load configuration from database
	config, err := core.LoadConfig(bridge)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Update bridge with loaded config
	if err := bridge.UpdateConfig(config); err != nil {
		log.Fatalf("Failed to update bridge config: %v", err)
	}

	// One-time backfill of Bambu tray spool assignments (Spoolman
	// extra.active_tray -> printer_slots.spool_id) after the v3 migration.
	// Retried on the next startup when Spoolman is unreachable.
	go func() {
		if err := bridge.BackfillTraySpoolAssignments(); err != nil {
			log.Printf("Warning: %v", err)
		}
	}()

	// Override port from DB config only when the flag was not set explicitly.
	// Explicit --port always wins (the Docker entrypoint pins the internal port).
	if !portFlagSet && config.WebPort != "" && config.WebPort != core.DefaultWebPort {
		*port = config.WebPort
	}

	addr := *host + ":" + *port

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start NFC session cleanup background task
	go func() {
		ticker := time.NewTicker(1 * time.Minute) // Clean up every minute
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := nfc.CleanupExpiredSessions(bridge); err != nil {
					log.Printf("Error cleaning up NFC sessions: %v", err)
				}
			case <-sigChan:
				return
			}
		}
	}()

	if *webOnly {
		// Run only web interface
		fmt.Println("Starting API server only...")
		webServer := server.NewWebServer(bridge)
		go func() {
			if err := webServer.Start(addr); err != nil {
				log.Fatalf("Web server error: %v", err)
			}
		}()

		<-sigChan
		fmt.Println("Shutting down web server...")

	} else if *bridgeOnly {
		// Run only bridge service
		fmt.Println("Starting bridge service only...")
		fmt.Printf("Monitoring printers: %v\n", getPrinterNames(config))
		fmt.Printf("Spoolman URL: %s\n", config.SpoolmanURL)
		fmt.Printf("Poll interval: %v\n", config.PollInterval)

		go func() {
			ticker := time.NewTicker(config.PollInterval)
			defer ticker.Stop()

			// Run initial check
			snapmaker.MonitorPrinters(bridge)

			for {
				select {
				case <-ticker.C:
					snapmaker.MonitorPrinters(bridge)
				case <-sigChan:
					return
				}
			}
		}()

		<-sigChan
		fmt.Println("Shutting down bridge service...")

	} else {
		// Run both bridge service and API server
		fmt.Println("Starting both bridge service and API server...")
		fmt.Printf("Monitoring printers: %v\n", getPrinterNames(config))
		fmt.Printf("Spoolman URL: %s\n", config.SpoolmanURL)
		fmt.Printf("Poll interval: %v\n", config.PollInterval)
		fmt.Printf("API: http://%s\n", addr)

		// Create web server first so we can pass it to monitoring
		webServer := server.NewWebServer(bridge)

		// Start bridge monitoring in a goroutine
		go func() {
			ticker := time.NewTicker(config.PollInterval)
			defer ticker.Stop()

			// Run initial check
			snapmaker.MonitorPrinters(bridge)
			// Broadcast initial status
			webServer.BroadcastStatus()

			for {
				select {
				case <-ticker.C:
					snapmaker.MonitorPrinters(bridge)
					// Broadcast status after each monitoring cycle
					webServer.BroadcastStatus()
				case <-sigChan:
					return
				}
			}
		}()

		// Start web server in a goroutine
		go func() {
			if err := webServer.Start(addr); err != nil {
				log.Fatalf("Web server error: %v", err)
			}
		}()

		<-sigChan
		fmt.Println("Shutting down services...")
	}
}

// getPrinterNames returns a slice of printer names from config.
func getPrinterNames(config *core.Config) []string {
	names := make([]string, 0, len(config.Printers))
	for name := range config.Printers {
		names = append(names, name)
	}
	return names
}
