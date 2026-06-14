# FilaBridge+

[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)
[![Go Version](https://img.shields.io/badge/Go-1.23-00ADD8?logo=go)](https://golang.org/)
[![Node.js](https://img.shields.io/badge/Node.js-22-339933?logo=node.js)](https://nodejs.org/)

A high-performance Go microservice with a modern Next.js web UI that bridges your 3D printers and **Spoolman** for (mostly) automatic filament inventory management.

> **This fork** is maintained by [Papai Nerd](https://github.com/doutorinfamous) at [github.com/doutorinfamous/filabridge-plus](https://github.com/doutorinfamous/filabridge-plus). It focuses on **Snapmaker** and **Bambu Lab** printers. **PrusaLink / Prusa printer support has been removed** — see [Acknowledgments](#acknowledgments) below.

## Acknowledgments

**FilaBridge** was originally created by **[needo37](https://github.com/needo37)** as an open-source bridge between [PrusaLink](https://help.prusa3d.com/article/prusa-link-and-prusa-connect_382798)-compatible printers and [Spoolman](https://github.com/Donkie/Spoolman). The original project lives at [github.com/needo37/filabridge](https://github.com/needo37/filabridge).

Thank you, **needo37**, for building the foundation — the Spoolman integration, NFC workflow, SQLite persistence, print-error handling, and the overall architecture that made this fork possible. This version retains the GPL v3 license and builds on that work with a rewritten front-end and new printer integrations.

The original author discontinued active development on the Prusa-focused codebase. This fork continues the project for different hardware:

| Integration | Status in this fork |
|-------------|---------------------|
| **Snapmaker U1** (Moonraker / Klipper) | Supported — direct API, G-code parsing |
| **Bambu Lab** (AMS + external spool) | Supported — via **Home Assistant** + [ha-bambulab](https://github.com/greghesp/ha-bambulab) |
| **PrusaLink / Prusa printers** | **Not supported** |

### The Problem

Running multiple 3D printers with Spoolman means keeping filament inventory in sync after every print. Multi-material jobs make manual updates tedious and error-prone. FilaBridge+ watches your printers, tracks which spools are loaded where, and debits Spoolman when prints finish — with NFC tags for quick assignments across toolheads, AMS slots, and storage locations.

## Features

- **Snapmaker U1 (Moonraker)**: Direct Moonraker API integration with G-code parsing for accurate per-toolhead usage
- **Bambu Lab (Home Assistant)**: AMS tray mapping, RFID auto-assignment, and webhook-based usage tracking through ha-bambulab
- **Real-time Dashboard**: Live printer status via WebSocket with polling fallback
- **Multi-Toolhead / Multi-Slot**: Moonraker toolheads and Bambu AMS slots (plus external spool)
- **Smart Usage Tracking**: G-code parsing (Snapmaker) and HA utility-meter webhooks (Bambu)
- **Print History**: Job log with consumption per spool
- **Persistent Storage**: SQLite for mappings, slots, locations, and history
- **High Performance**: Lightweight Go backend + Next.js UI in a single container or binary workflow
- **Web-based Config**: Spoolman, Home Assistant, printers, and behavior — all in Settings
- **Smart Spool Search**: Filter spools by ID, material, brand, or name
- **Error Handling**: Print processing errors with acknowledge / resolve flow
- **NFC Tag Support**: QR codes and NFC tags for spools, filaments, toolheads, AMS slots, and custom locations
- **NFC Direct Write**: Only available in some devices with Chrome on Android.
- **Location Tracking**: Dryboxes, shelves, toolheads, and AMS slots

## Supported Printer Integrations

### Snapmaker U1 (Moonraker)

Connect directly to the printer's **Moonraker** instance. FilaBridge+ polls job state, downloads G-code on completion, and calculates filament usage per extruder/toolhead.

- Add the printer in **Settings → Printers → Snapmaker U1**
- Map Spoolman spools to toolheads on the dashboard
- Usage is debited when the job reaches a completed state

Other Klipper/Moonraker printers may work with the same driver; **Snapmaker U1** is the tested target.

### Bambu Lab (Home Assistant)

Bambu printers are integrated **indirectly** through **Home Assistant** and the community **[ha-bambulab](https://github.com/greghesp/ha-bambulab)** add-on (install via HACS). FilaBridge+ does not talk to the printer directly — it discovers printers from HA, generates YAML automation packages, and receives webhooks for spool usage and tray changes.

1. Install **ha-bambulab** in Home Assistant and add your printer (LAN or Cloud)
2. Configure **HA URL**, **token**, and **FilaBridge+ public URL** in **Settings → General → Home Assistant**
3. Register the printer under **Settings → Printers → Bambu Lab (HA)**
4. Download the **HA package** YAML, place it in HA `packages/`, and restart Home Assistant
5. Map AMS slots to Spoolman spools on the dashboard or via NFC

Full step-by-step guide: **[docs/home-assistant-setup.md](docs/home-assistant-setup.md)**

## Prerequisites

- **[Spoolman](https://github.com/Donkie/Spoolman)** on your network
- **For Snapmaker**: Moonraker-enabled printer (Snapmaker U1 recommended)
- **For Bambu Lab**:
  - [Home Assistant](https://www.home-assistant.io/)
  - [HACS](https://hacs.xyz/)
  - [ha-bambulab](https://github.com/greghesp/ha-bambulab) integration
- **For building from source**: Go 1.23+, Node.js 20+
- **(Optional) NFC**: NFC-capable phone and NTAG213/215/216 tags; **NFC Tools Pro** (or similar) to program tags

## Installation

### Option 1: Docker (recommended)

1. **Run Spoolman** (if not already running):

   ```bash
   docker run -d --name spoolman -p 8000:8000 -v spoolman-data:/home/spoolman/data ghcr.io/donkie/spoolman:latest
   ```

2. **Clone and start FilaBridge+**:

   ```bash
   git clone https://github.com/doutorinfamous/filabridge-plus.git
   cd filabridge-plus
   docker compose up -d --build
   ```

3. **Configure**: Open `http://localhost:5000` → **Settings**

**Spoolman on the Docker host:** If Spoolman runs outside the FilaBridge+ container, set the Spoolman URL in the web UI to `http://host.docker.internal:8000` (adjust the port if needed). Do **not** use `localhost` — inside the container that address points to FilaBridge+ itself. The compose file includes `extra_hosts: host.docker.internal:host-gateway` for Linux; Docker Desktop on Windows/macOS provides this hostname by default.

The database persists in the `filabridge_data` Docker volume (`FILABRIDGE_DB_PATH=/app/data`).

## Configuration

All settings are stored in SQLite (`filabridge.db`). In Docker, set `FILABRIDGE_DB_PATH` to control the data directory (default `/app/data`).

### First run

1. Start FilaBridge+ and open `http://localhost:5000`
2. **Settings → Spoolman** — enter Spoolman URL and test the connection
3. **Settings → Home Assistant** — required for Bambu; enter HA URL, long-lived token, and the URL HA uses to reach FilaBridge+ (must be a LAN IP, not `localhost`, when HA is on another machine)
4. **Settings → Printers** — add a Snapmaker U1 (Moonraker host) and/or register a Bambu printer from HA discovery
5. **Dashboard** — map Spoolman spools to toolheads or AMS slots

For Bambu, after registering a printer, download the HA package, install it in Home Assistant, restart HA, and use **Validate HA** in Settings.

## Usage

### Web interface

| Page | Purpose |
|------|---------|
| **Dashboard** | Printer status, toolhead / AMS mapping, print errors, connection health |
| **History** | Completed and failed jobs with filament consumption |
| **NFC & QR** | Generate tags for spools, filaments, locations, toolheads, and AMS slots |
| **Settings** | Spoolman, Home Assistant, printers, polling/timeouts, database browser |

Real-time updates use WebSocket (`/ws/status`); the UI falls back to polling if the socket drops.

### Filament workflow

1. Add spools in **Spoolman**
2. Map spools to Moonraker toolheads or Bambu AMS slots in FilaBridge+
3. Print — usage is tracked automatically on job completion (Snapmaker) or via HA webhooks (Bambu)
4. Acknowledge any processing errors shown on the dashboard

### NFC workflow

1. Generate QR/NFC URLs on **NFC & QR**
2. Program tags with NFC Tools Pro (or similar) or direct via browser on supported Chrome on Android devices.
3. Scan **spool** then **location** (or the reverse) on `/nfc/scan` — toolheads, AMS slots, and custom locations are supported
4. Sessions expire after 5 minutes; complete both scans within the timeout

## API Endpoints

The Next.js server on port **5000** is the single entry point: it serves the UI and proxies `/api/*` and `/ws/*` to the Go backend on **5001**.

### Core

- `GET /api/status` — Printer status, mappings, and health
- `GET /api/spools` — All spools from Spoolman
- `GET /api/filaments` — All filament types from Spoolman
- `POST /api/map_toolhead` — Map a spool to a Moonraker toolhead
- `GET /api/available_spools` — Spools available for assignment
- `GET|POST /api/spoolman/test` — Test Spoolman connection
- `GET /api/config` / `POST /api/config` — Global configuration
- `GET|POST /api/printers` — List / add printers
- `GET /api/print-errors` — Unacknowledged print errors
- `POST /api/print-errors/{id}/acknowledge` — Acknowledge error
- `POST /api/print-errors/{id}/resolve` — Resolve error
- `GET /api/history/jobs` — Print history list
- `GET /api/history/jobs/{id}` — Single job detail
- `WS /ws/status` — Real-time status WebSocket

### Home Assistant & Bambu

- `GET|POST /api/ha/test` — Test HA connection
- `GET|POST /api/ha/config` — HA settings
- `GET /api/ha/printers` — Discover Bambu printers from HA
- `POST /api/ha/printers` — Register a Bambu printer
- `GET /api/ha/automations/{id}` — Download HA package YAML
- `GET /api/ha/validate/{id}` — Validate required HA entities
- `POST /api/trays/assign` — Assign spool to AMS slot
- `GET|POST /api/webhook` — Bambu/HA webhook receiver

### NFC & locations

- `GET /api/nfc/assign` — NFC scan handler (spool or location)
- `GET /api/nfc/urls` — NFC URLs with QR data
- `GET /api/nfc/session/status` — Active NFC session
- `POST /api/nfc/session/select-spool` — Pick spool during NFC flow
- `GET|POST /api/locations` — Custom storage locations
- `GET /api/locations/{name}/status` — Location occupancy

## Project Structure

```
filabridge/
├── backend/               # Go API (internal port 5001)
│   ├── main.go            # Entry point (--web-only / --bridge-only / --port)
│   ├── core/              # Bridge, SQLite, config, history, filament accounting
│   ├── snapmaker/         # Snapmaker U1: Moonraker client, G-code, monitor
│   ├── bambu/             # Bambu Lab: HA discovery, AMS trays, webhooks, YAML
│   ├── spoolman/          # Spoolman API client
│   ├── homeassistant/     # Home Assistant REST client
│   ├── nfc/               # NFC sessions (spool + location)
│   └── server/            # HTTP API (Gin) + WebSocket /ws/status
├── web/                   # Next.js + shadcn/ui front-end (port 5000)
│   ├── app/               # Dashboard, History, NFC, Settings + /api proxy
│   ├── components/        # Printer cards, comboboxes, settings forms
│   └── lib/               # API client, types, WebSocket hook
├── docs/                  # Setup guides (home-assistant-setup.md)
├── docker/entrypoint.sh   # Runs Go (5001) + Next.js (5000) in one container
├── Dockerfile
└── docker-compose.yml
```

## Troubleshooting

### Snapmaker / Moonraker

- Verify Moonraker host/IP in **Settings → Printers**
- Ensure spools are mapped to toolheads before printing
- Prints must reach a **completed** state for G-code parsing to run
- Check backend logs for `Print finished detected` and G-code download errors

### Bambu Lab / Home Assistant

- Confirm **ha-bambulab** is installed and the printer appears in HA
- **FilaBridge+ public URL** must be reachable from HA (use LAN IP, not `localhost`)
- After updating the HA package, **restart Home Assistant** and run **Validate HA**
- Required entities: `sensor.filabridge_*_filament_usage`, `*_filament_usage_meter`, `input_number.filabridge_*_last_tray`, `sensor.filabridge_*_active_tray`
- See **[docs/home-assistant-setup.md](docs/home-assistant-setup.md)** for webhook tests and `utility_meter.calibrate` issues

### Spoolman

- Confirm Spoolman is running and the URL is correct
- In Docker, use `http://host.docker.internal:PORT` when Spoolman runs on the host

### WebSocket

- Check the browser console for connection errors
- The UI polls automatically if WebSocket fails

### NFC

- Use NTAG213, NTAG215, or NTAG216 tags
- QR codes encode full URLs — scan with your NFC app to write tags
- Complete both scans within the 5-minute session timeout

## Development

```bash
# Backend (Go API at http://localhost:5001)
cd backend
go build ./...
go test ./...
go run . --port 5001

# Front-end (UI at http://localhost:5000, proxies to the API)
cd web
npm install
npm run dev -- -p 5000
```

Open `http://localhost:5000`. Override the backend with `BACKEND_URL` (default `http://127.0.0.1:5001`).

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## Roadmap

- [x] Docker image (Go + Next.js single container)
- [x] Real-time WebSocket updates
- [x] NFC support (spools, locations, AMS slots)
- [x] Snapmaker U1 / Moonraker integration
- [x] Bambu Lab via Home Assistant + ha-bambulab
- [x] Print history
- [x] Modern Next.js + shadcn/ui front-end
- [ ] Mobile-responsive UI polish
- [ ] Broader Moonraker/Klipper printer testing
- [ ] OpenAPI / Swagger documentation

## Contributing

Contributions are welcome!

- Report bugs and suggest features via GitHub Issues
- Submit focused PRs (see [CONTRIBUTING.md](CONTRIBUTING.md))
- Improve docs — especially HA setup and printer-specific guides

## License

This project is licensed under the **GNU General Public License v3.0** — see [LICENSE](LICENSE). As a fork of [needo37/filabridge](https://github.com/needo37/filabridge), derivative works remain under GPL v3.

## Support

| Topic | Where to look |
|-------|----------------|
| **This fork (Snapmaker / Bambu / Spoolman bridge)** | [Issues](https://github.com/doutorinfamous/filabridge-plus/issues) on this repository |
| **Original FilaBridge+ (PrusaLink era)** | [needo37/filabridge](https://github.com/needo37/filabridge) |
| **Spoolman** | [Donkie/Spoolman](https://github.com/Donkie/Spoolman) |
| **Bambu in Home Assistant** | [greghesp/ha-bambulab](https://github.com/greghesp/ha-bambulab) |
| **Moonraker / Klipper** | [Moonraker docs](https://moonraker.readthedocs.io/) |
