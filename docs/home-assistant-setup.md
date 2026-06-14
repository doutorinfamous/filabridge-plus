# Home Assistant Setup — Bambu Lab + FilaBridge+

This guide explains how to connect **Bambu Lab** printers to FilaBridge+ via **Home Assistant** and the **ha-bambulab** integration, replicating the SpoolmanSync workflow inside FilaBridge+.

## Prerequisites

- [Spoolman](https://github.com/Donkie/Spoolman) reachable on your network
- [Home Assistant](https://www.home-assistant.io/) running
- [HACS](https://hacs.xyz/) installed in HA
- **[ha-bambulab](https://github.com/greghesp/ha-bambulab)** integration installed via HACS
- Bambu printer added in Home Assistant (LAN or Cloud)
- FilaBridge+ reachable by HA on the local network

## 1. Install ha-bambulab in Home Assistant

1. Open HACS → Integrations → search for **Bambu Lab**
2. Install **ha-bambulab**
3. Restart Home Assistant
4. Go to **Settings → Devices & services → Add integration → Bambu Lab**
5. Follow the wizard (LAN or Bambu Cloud account)
6. Confirm entities appear such as:
   - `sensor.<prefix>_print_status`
   - `sensor.<prefix>_tray_1` … `tray_4`
   - `sensor.<prefix>_external_spool` (if applicable)

## 2. Configure FilaBridge+

1. Open FilaBridge+ → **Settings → General**
2. In the **Home Assistant** section:
   - **HA URL**: `http://HA_IP:8123`
   - **HA Token**: create a Long-Lived Access Token in HA → Profile → Long-Lived Access Tokens
   - **FilaBridge+ Public URL**: URL HA can reach, e.g. `http://192.168.1.20:5000`
3. Click **Test connection** (uses form values, even before saving)
4. Click **Save** to persist

> **Important:** If HA runs on another machine, do not use `localhost` for the FilaBridge+ public URL.

## 3. Register Bambu printer in FilaBridge+

1. **Settings → Printers → Bambu Lab (HA)**
2. Select the discovered printer
3. The printer appears on the dashboard under Bambu Lab printers

## 4. Generate and install HA automations

1. On the dashboard or in Settings → Printers, click **HA package**
2. Save the downloaded YAML to `config/packages/filabridge_<prefix>.yaml` in Home Assistant  
   The filename is **always lowercase** (e.g. `filabridge_03919c461204338.yaml`, not `filabridge_03919C461204338.yaml`)
3. In `configuration.yaml`, ensure:

```yaml
homeassistant:
  packages: !include_dir_named packages
```

4. **Restart Home Assistant** (required after utility_meter and template sensors)
5. In FilaBridge+, click **Validate HA** on the Bambu printer (or check manually in **Developer Tools → States**):
   - `sensor.filabridge_<prefix>_filament_usage`
   - `sensor.filabridge_<prefix>_filament_usage_meter` — **required**; without it, `utility_meter.calibrate` is unknown
   - `input_number.filabridge_<prefix>_last_tray`
   - `sensor.filabridge_<prefix>_active_tray`

## 5. Map spools (Spoolman)

### Via the UI

On the **Bambu Lab Printers** dashboard, use the dropdown on each AMS slot to assign a Spoolman spool.

### Via NFC (FilaBridge+ flow)

1. Generate an NFC tag for the **spool** on the NFC tab
2. Generate an NFC tag for the **AMS slot** (AMS Slots section)
3. Scan: spool first, then the slot
4. FilaBridge+ stores `spool_id` on the slot (`printer_slots` table) and mirrors `extra.active_tray` in Spoolman

AMS location format:

```
{Printer Name} - AMS 1 Slot 2
{Printer Name} - External Spool
```

## 6. How automatic tracking works

| HA event | FilaBridge+ webhook | Action |
|----------|-------------------|--------|
| Print start | `print_started` | Opens a print history job (file + printer) |
| Print end / tray change | `spool_usage` | Debits weight from the spool on the active tray and logs usage on the open job |
| Print end | `print_finished` | Closes the history job (`finish` = completed; other state = failed) |
| Physical spool swap (RFID) | `tray_change` | Auto-assigns spool from learned `extra.tag` |
| Empty tray (`name=Empty`) | `tray_change` | Unassigns spool from tray |

Spool ↔ tray mapping lives in the FilaBridge+ database (`printer_slots` table, `spool_id` column — same table used by Moonraker toolheads). Spoolman gets a mirror in `extra.active_tray` (value = HA entity `unique_id`) for display.

> **Important:** `print_started`/`print_finished` events and history logging require the updated YAML package. If you installed the package before this version, **regenerate HA Config in FilaBridge+ and replace the file in `packages/`**, then restart HA. With old YAML, Spoolman debit still works, but history groups consumption into auto-created jobs without file names.

## 7. Testing

1. Assign a spool to an AMS slot
2. Start a short print
3. When finished, verify weight was debited in Spoolman
4. Swap a spool with a Bambu RFID tag — should reassign automatically

### Manual test from Home Assistant (without printing)

FilaBridge+ package `rest_command` services are **fixed** (they do not include the printer prefix):

| Service | Use |
|---------|-----|
| `rest_command.filabridge_update_spool` | Simulate filament debit |
| `rest_command.filabridge_tray_change` | Simulate spool swap |

**Developer Tools → Actions** — example to debit 5.5 g from slot 3:

- **Action:** `rest_command.filabridge_update_spool`
- **YAML data** (adjust tray `entity_id`):

```yaml
filament_name: "HA test"
filament_material: "PLA"
filament_tray_uuid: ""
filament_used_weight: "5.5"
filament_color: "#FFFFFF"
filament_active_tray_id: "sensor.bambu_lab_a1_ams_tray_3"
```

Reusable script (HA script editor — **no** key at the top, start with `alias:`):

```yaml
alias: "FilaBridge test - debit Slot 3"
icon: mdi:printer-3d-nozzle
mode: single
sequence:
  - action: rest_command.filabridge_update_spool
    data:
      filament_name: "HA test"
      filament_material: "PLA"
      filament_tray_uuid: ""
      filament_used_weight: "5.5"
      filament_color: "#FFFFFF"
      filament_active_tray_id: "sensor.bambu_lab_a1_ams_tray_3"
```

Printer-prefixed entities (e.g. `03919c461204338`): `sensor.filabridge_<prefix>_filament_usage_meter`, `input_number.filabridge_<prefix>_last_tray` — **do not** confuse with `rest_command` names.

## Troubleshooting

### `utility_meter.calibrate` — unknown action

The `filabridge_update_spool_*` automation uses `utility_meter.calibrate` to reset the accumulator after debiting filament in Spoolman (same logic as SpoolmanSync). **Do not remove this action** — it only appears as unknown when the utility meter was not loaded in HA.

| Symptom | Cause | Fix |
|---------|-------|-----|
| Only `filament_usage` exists, not `filament_usage_meter` | Incomplete package or HA not restarted | Download **HA Config** again, replace the full file in `packages/`, restart HA |
| Warning when print starts | Invalid automation while meter is missing | After fixing the package, the warning clears |
| Old package with `cycle: none` | Invalid `utility_meter` value (bug fixed in SpoolmanSync v1.2.0) | Regenerate YAML in FilaBridge+ (current version omits `cycle`) |

Post-restart checklist: use **Validate HA** in FilaBridge+ or confirm the 4 entities listed in section 4.

### Webhook does not reach FilaBridge+

- Test from HA: `curl -X POST http://FILABRIDGE_IP:5000/api/webhook -H "Content-Type: application/json" -d '{"event":"spool_usage","active_tray_id":"test","used_weight":0}'`
- Confirm **FilaBridge+ Public URL** uses a network IP, not `localhost`
- Check firewall between HA and FilaBridge+

### No printers discovered

- Confirm ha-bambulab is installed and the printer is visible in HA
- Valid HA token with read permissions
- Test connection in Settings

### Weight not debited

- Verify `filabridge_update_spool_*` automations are active in HA
- Confirm spool assigned to slot on the FilaBridge+ dashboard (mirror `extra.active_tray` appears in Spoolman)
- Check HA logs in **Settings → System → Logs**

The automation **does not call the webhook during the print** — only at the **end** (`print_status` → `finish`/`idle`) or on **tray change**. During printing, monitor these sensors:

| Sensor | Expected behavior |
|--------|-------------------|
| `sensor.filabridge_<prefix>_filament_usage` | Increases with progress |
| `sensor.filabridge_<prefix>_filament_usage_meter` | Accumulates grams |
| `sensor.bambu_lab_a1_print_weight` | > 0 (total job weight) |
| `sensor.bambu_lab_a1_print_progress` | Rises from 0 → 100 |
| `sensor.filabridge_<prefix>_active_tray` | Shows active slot (e.g. `13` = AMS slot 3) |

If `print_weight` stays **0**, the meter never accumulates and debit is skipped (`tray_weight >= 0.01`).

At print end, the automation only debits if `print_status` goes from `running`/`pause`/etc. → `finish`/`idle`. Regenerating the YAML package in FilaBridge+ fixes stale `last_tray` and the `print_start` trigger (records slot on start).

### RFID does not auto-assign

- Only works with Bambu spools with valid RFID tags
- Spools without RFID report all-zero `tray_uuid` — ignored by FilaBridge+
- `extra.tag` is learned on the first `spool_usage` with valid RFID

### Coexistence with Moonraker (Snapmaker)

Moonraker printers keep G-code polling. Bambu printers use HA webhooks only — no conflict.
