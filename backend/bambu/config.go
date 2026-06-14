package bambu

import (
	"fmt"
	"strings"
)

const filabridgeEntityPrefix = "filabridge_"

// GenerateHAPackage generates a complete HA package YAML for one Bambu printer.
func GenerateHAPackage(printer Printer, webhookURL string) string {
	printer.Prefix = NormalizePrefix(printer.Prefix)
	allTrays := CollectTrayInfos(printer)
	automations := generateAutomationsYAML(printer.Prefix, allTrays, webhookURL, printer)
	config := generateConfigurationYAML(printer.Prefix, allTrays, printer, webhookURL)
	return fmt.Sprintf("# FilaBridge HA package for printer: %s\n\n%s\n\n%s", printer.Name, config, automations)
}

func buildTrayEntityLookup(allTrays []TrayInfo) string {
	var parts []string
	for _, tray := range allTrays {
		// Go fmt: %% → literal %. Jinja needs {% if ... %} not {%%% if ... %}
		parts = append(parts, fmt.Sprintf("{%% if tray_composite == %d %%}%s{%% endif %%}", tray.CompositeID, tray.EntityID))
	}
	return strings.Join(parts, "")
}

func buildActiveTrayDetection(allTrays []TrayInfo) string {
	if len(allTrays) == 0 {
		return "-1"
	}
	activeCheck := "state_attr('%s', 'active') in [true, 'true', 'True']"
	expr := "-1"
	for i := len(allTrays) - 1; i >= 0; i-- {
		t := allTrays[i]
		check := fmt.Sprintf(activeCheck, t.EntityID)
		expr = fmt.Sprintf("(%d if %s else %s)", t.CompositeID, check, expr)
	}
	return expr
}

// buildJobNameTemplate returns a Jinja expression for the current print job name.
// ha-bambulab exposes subtask_name as its own sensor (friendly name "Task name"),
// not as an attribute on print_status.
func buildJobNameTemplate(taskNameEntity, gcodeFileEntity string) string {
	if taskNameEntity != "" && gcodeFileEntity != "" {
		return fmt.Sprintf(
			"states('%s') | default(states('%s'), true) | default('', true)",
			taskNameEntity, gcodeFileEntity,
		)
	}
	if taskNameEntity != "" {
		return fmt.Sprintf("states('%s') | default('', true)", taskNameEntity)
	}
	if gcodeFileEntity != "" {
		return fmt.Sprintf("states('%s') | default('', true)", gcodeFileEntity)
	}
	return "''"
}

func buildFilamentUsageTemplate(printWeightEntity, printProgressEntity string) string {
	if printWeightEntity == "" || printProgressEntity == "" {
		return "0"
	}
	// Single-line template: multi-line {% set %} breaks YAML block scalars (unindented lines).
	return fmt.Sprintf(
		"{{ (states('%s') | float(0)) * ((states('%s') | float(0) / 100.0) if (states('%s') | float(0) > 1) else (states('%s') | float(0))) | round(3) }}",
		printWeightEntity, printProgressEntity, printProgressEntity, printProgressEntity,
	)
}

func generateConfigurationYAML(prefix string, allTrays []TrayInfo, printer Printer, webhookURL string) string {
	maxComposite := 99
	for _, t := range allTrays {
		if t.CompositeID > maxComposite {
			maxComposite = t.CompositeID
		}
	}

	availabilityEntities := make([]string, len(allTrays))
	for i, t := range allTrays {
		availabilityEntities[i] = fmt.Sprintf("'%s'", t.EntityID)
	}

	activeTrayDetection := buildActiveTrayDetection(allTrays)

	return fmt.Sprintf(`# FilaBridge configuration additions for %s
input_number:
  %s%s_last_tray:
    name: "FilaBridge %s Last Tray"
    min: 0
    max: %d
    step: 1

utility_meter:
  %s%s_filament_usage_meter:
    unique_id: filabridge-%s-filament-usage-meter
    source: sensor.%s%s_filament_usage

rest_command:
  %supdate_spool:
    url: "%s"
    method: POST
    headers:
      Content-Type: "application/json"
    payload: >
      {
        "event": "spool_usage",
        "name": "{{ filament_name }}",
        "material": "{{ filament_material }}",
        "tray_uuid": "{{ filament_tray_uuid }}",
        "used_weight": {{ filament_used_weight | default(0) | round(2) }},
        "color": "{{ filament_color }}",
        "active_tray_id": "{{ filament_active_tray_id }}",
        "printer_prefix": "{{ printer_prefix | default('') }}",
        "job_name": "{{ job_name | default('') }}"
      }

  %stray_change:
    url: "%s"
    method: POST
    headers:
      Content-Type: "application/json"
    payload: >
      {
        "event": "tray_change",
        "tray_entity_id": "{{ tray_entity_id }}",
        "tray_uuid": "{{ tray_uuid }}",
        "name": "{{ name }}",
        "material": "{{ material }}",
        "color": "{{ color }}"
      }

  %sprint_event:
    url: "%s"
    method: POST
    headers:
      Content-Type: "application/json"
    payload: >
      {
        "event": "{{ event_type }}",
        "printer_prefix": "{{ printer_prefix | default('') }}",
        "job_name": "{{ job_name | default('') }}",
        "print_state": "{{ print_state | default('') }}"
      }

template:
  - sensor:
      - name: "FilaBridge %s Filament Usage"
        unique_id: filabridge-%s-filament-usage
        state: >
          %s
        availability: >
          {{ states('%s') not in ['unknown', 'unavailable']
             and states('%s') not in ['unknown', 'unavailable'] }}

      - name: "FilaBridge %s Active Tray"
        unique_id: filabridge-%s-active-tray
        state: "{{ %s }}"
        availability: >
          {{ expand([
            %s
          ]) | rejectattr('state', 'eq', 'unavailable') | list | count > 0 }}
`,
		prefix,
		filabridgeEntityPrefix, prefix, prefix, maxComposite,
		filabridgeEntityPrefix, prefix, prefix, filabridgeEntityPrefix, prefix,
		filabridgeEntityPrefix, webhookURL,
		filabridgeEntityPrefix, webhookURL,
		filabridgeEntityPrefix, webhookURL,
		prefix, prefix,
		buildFilamentUsageTemplate(printer.PrintWeightEntity, printer.PrintProgressEntity),
		printer.PrintWeightEntity, printer.PrintProgressEntity,
		prefix, prefix, activeTrayDetection,
		strings.Join(availabilityEntities, ",\n            "),
	)
}

func generateAutomationsYAML(prefix string, allTrays []TrayInfo, webhookURL string, printer Printer) string {
	trayEntityLookup := buildTrayEntityLookup(allTrays)
	var trayEntityIDs strings.Builder
	for i, t := range allTrays {
		if i > 0 {
			trayEntityIDs.WriteString("\n")
		}
		trayEntityIDs.WriteString(fmt.Sprintf("        - %s", t.EntityID))
	}

	printEndEntity := printer.EntityID
	if printEndEntity == "" {
		printEndEntity = printer.CurrentStageEntity
	}
	jobNameTemplate := buildJobNameTemplate(printer.TaskNameEntity, printer.GcodeFileEntity)

	return fmt.Sprintf(`automation:
  - id: 'filabridge_update_spool_%s'
    alias: FilaBridge - Update Spool (%s)
    description: Track spool usage and sync with FilaBridge/Spoolman
    triggers:
      - entity_id: sensor.%s%s_active_tray
        id: tray
        trigger: state
      - entity_id: %s
        to: running
        id: print_start
        trigger: state
      - entity_id: %s
        to:
          - finish
          - idle
        id: print_end
        trigger: state
    variables:
      old_tray: |-
        {%% if trigger.id == 'tray' and trigger.from_state is not none and trigger.from_state.state not in [None, '', 'unknown', 'unavailable'] %%}
          {{ trigger.from_state.state | int(-1) }}
        {%% else %%}
          -1
        {%% endif %%}
      new_tray: |-
        {%% if trigger.id == 'tray' and trigger.to_state is not none and trigger.to_state.state not in [None, '', 'unknown', 'unavailable'] %%}
          {{ trigger.to_state.state | int(-1) }}
        {%% else %%}
          -1
        {%% endif %%}
      tray_composite: |-
        {%% if trigger.id == 'print_end' %%}
          {%% set active_tray = states('sensor.%s%s_active_tray') | int(-1) %%}
          {{ active_tray if active_tray >= 0 else states('input_number.%s%s_last_tray') | int(-1) }}
        {%% else %%}
          {{ old_tray }}
        {%% endif %%}
      tray_sensor: "%s"
      tray_weight: "{{ states('sensor.%s%s_filament_usage_meter') | float(0) | round(2) }}"
      tray_uuid: "{{ state_attr(tray_sensor, 'tray_uuid') | default('') }}"
      material: "{{ state_attr(tray_sensor, 'type') | default('') }}"
      name: "{{ state_attr(tray_sensor, 'name') | default('') }}"
      color: "{{ state_attr(tray_sensor, 'color') | default('') }}"
      printer_prefix: "%s"
      job_name: "{{ %s }}"
    actions:
      - choose:
          - conditions:
              - condition: template
                value_template: "{{ trigger.id == 'print_start' }}"
            sequence:
              - action: input_number.set_value
                target:
                  entity_id: input_number.%s%s_last_tray
                data:
                  value: "{{ states('sensor.%s%s_active_tray') | int(-1) }}"
              - action: rest_command.%sprint_event
                data:
                  event_type: print_started
                  printer_prefix: "{{ printer_prefix }}"
                  job_name: "{{ job_name }}"
                  print_state: ""
          - conditions:
              - condition: template
                value_template: "{{ trigger.id == 'tray' }}"
            sequence:
              - choose:
                  - conditions:
                      - condition: template
                        value_template: "{{ old_tray >= 0 and tray_weight >= 0.01 and tray_sensor != '' }}"
                    sequence:
                      - action: rest_command.%supdate_spool
                        data:
                          filament_name: "{{ name }}"
                          filament_material: "{{ material }}"
                          filament_tray_uuid: "{{ tray_uuid }}"
                          filament_used_weight: "{{ tray_weight }}"
                          filament_color: "{{ color }}"
                          filament_active_tray_id: "{{ tray_sensor }}"
                          printer_prefix: "{{ printer_prefix }}"
                          job_name: "{{ job_name }}"
                      - action: utility_meter.calibrate
                        target:
                          entity_id: sensor.%s%s_filament_usage_meter
                        data:
                          value: "0"
              - action: utility_meter.calibrate
                target:
                  entity_id: sensor.%s%s_filament_usage_meter
                data:
                  value: "0"
              - action: input_number.set_value
                target:
                  entity_id: input_number.%s%s_last_tray
                data:
                  value: "{{ new_tray }}"
          - conditions:
              - condition: template
                value_template: >-
                  {{ trigger.id == 'print_end'
                     and trigger.from_state is not none
                     and trigger.from_state.state in ['running', 'pause', 'prepare', 'slicing', 'failed'] }}
            sequence:
              - choose:
                  - conditions:
                      - condition: template
                        value_template: "{{ tray_composite >= 0 and tray_weight >= 0.01 and tray_sensor != '' }}"
                    sequence:
                      - action: rest_command.%supdate_spool
                        data:
                          filament_name: "{{ name }}"
                          filament_material: "{{ material }}"
                          filament_tray_uuid: "{{ tray_uuid }}"
                          filament_used_weight: "{{ tray_weight }}"
                          filament_color: "{{ color }}"
                          filament_active_tray_id: "{{ tray_sensor }}"
                          printer_prefix: "{{ printer_prefix }}"
                          job_name: "{{ job_name }}"
              - action: utility_meter.calibrate
                target:
                  entity_id: sensor.%s%s_filament_usage_meter
                data:
                  value: "0"
              - action: rest_command.%sprint_event
                data:
                  event_type: print_finished
                  printer_prefix: "{{ printer_prefix }}"
                  job_name: "{{ job_name }}"
                  print_state: "{{ trigger.to_state.state }}"
    mode: single

  - id: 'filabridge_tray_change_%s'
    alias: FilaBridge - Tray Change (%s)
    description: Detect physical spool changes and auto-assign/unassign
    triggers:
      - entity_id:
%s
        trigger: state
      - entity_id:
%s
        attribute: tray_uuid
        trigger: state
      - entity_id:
%s
        attribute: name
        trigger: state
    conditions:
      - condition: template
        value_template: "{{ trigger.to_state is not none and trigger.to_state.state not in ['unavailable', 'unknown'] }}"
      - condition: template
        value_template: >-
          {{ trigger.from_state is none or trigger.to_state is none or
             trigger.to_state.attributes.get('tray_uuid', '') != trigger.from_state.attributes.get('tray_uuid', '') or
             trigger.to_state.attributes.get('name', '') != trigger.from_state.attributes.get('name', '') }}
    variables:
      tray_entity_id: "{{ trigger.entity_id }}"
      tray_uuid: "{{ state_attr(trigger.entity_id, 'tray_uuid') | default('') }}"
      name: "{{ state_attr(trigger.entity_id, 'name') | default('') }}"
      material: "{{ state_attr(trigger.entity_id, 'type') | default('') }}"
      color: "{{ state_attr(trigger.entity_id, 'color') | default('') }}"
    actions:
      - action: rest_command.%stray_change
        data:
          tray_entity_id: "{{ tray_entity_id }}"
          tray_uuid: "{{ tray_uuid }}"
          name: "{{ name }}"
          material: "{{ material }}"
          color: "{{ color }}"
    mode: queued
    max: 10
`,
		prefix, prefix,
		filabridgeEntityPrefix, prefix,
		printEndEntity,
		printEndEntity,
		filabridgeEntityPrefix, prefix, filabridgeEntityPrefix, prefix,
		trayEntityLookup,
		filabridgeEntityPrefix, prefix,
		prefix,
		jobNameTemplate,
		filabridgeEntityPrefix, prefix, filabridgeEntityPrefix, prefix,
		filabridgeEntityPrefix,
		filabridgeEntityPrefix,
		filabridgeEntityPrefix, prefix,
		filabridgeEntityPrefix, prefix,
		filabridgeEntityPrefix, prefix,
		filabridgeEntityPrefix,
		filabridgeEntityPrefix, prefix,
		filabridgeEntityPrefix,
		prefix, prefix,
		trayEntityIDs.String(),
		trayEntityIDs.String(),
		trayEntityIDs.String(),
		filabridgeEntityPrefix,
	)
}
