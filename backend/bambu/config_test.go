package bambu

import (
	"strings"
	"testing"
)

func TestBuildActiveTrayDetectionSingleLine(t *testing.T) {
	trays := []TrayInfo{
		{EntityID: "sensor.bambu_lab_a1_ams_tray_3", CompositeID: 13},
		{EntityID: "sensor.bambu_lab_a1_ams_tray_1", CompositeID: 11},
	}
	got := buildActiveTrayDetection(trays)
	if strings.Contains(got, "{%") {
		t.Fatalf("expected expression without block tags, got %q", got)
	}
	if !strings.Contains(got, "sensor.bambu_lab_a1_ams_tray_3") || !strings.Contains(got, " else -1") {
		t.Fatalf("unexpected active tray expression: %q", got)
	}
}

func TestBuildTrayEntityLookupJinjaSyntax(t *testing.T) {
	trays := []TrayInfo{
		{EntityID: "sensor.bambu_lab_a1_external_pool_external_spool", CompositeID: 0},
		{EntityID: "sensor.bambu_lab_a1_ams_tray_1", CompositeID: 11},
	}
	got := buildTrayEntityLookup(trays)

	want0 := "{% if tray_composite == 0 %}sensor.bambu_lab_a1_external_pool_external_spool{% endif %}"
	want1 := "{% if tray_composite == 11 %}sensor.bambu_lab_a1_ams_tray_1{% endif %}"

	if !strings.Contains(got, want0) {
		t.Errorf("missing external spool jinja block.\ngot:  %q\nwant: %q", got, want0)
	}
	if !strings.Contains(got, want1) {
		t.Errorf("missing AMS tray jinja block.\ngot:  %q\nwant: %q", got, want1)
	}
	if strings.Contains(got, "%!") {
		t.Errorf("fmt.Sprintf corruption in jinja output: %q", got)
	}
	if strings.Contains(got, "{%%%") {
		t.Errorf("invalid triple-percent jinja in output: %q", got)
	}
}

func TestGenerateAutomationsTrayChangeAttributeTriggers(t *testing.T) {
	printer := Printer{
		Prefix: "03919c461204338",
		Name:   "Test",
		AMSUnits: []AMS{{
			AMSNumber: 1,
			Trays: []Tray{{
				EntityID:   "sensor.bambu_lab_a1_ams_tray_1",
				TrayNumber: 1,
				AMSNumber:  1,
			}},
		}},
	}
	yaml := generateAutomationsYAML(printer.Prefix, CollectTrayInfos(printer), "http://filabridge:5000/api/webhook", printer)

	for _, want := range []string{
		"attribute: tray_uuid",
		"attribute: name",
		"sensor.bambu_lab_a1_ams_tray_1",
	} {
		if !strings.Contains(yaml, want) {
			t.Errorf("generated automations missing %q", want)
		}
	}
}

func TestBuildFilamentUsageTemplateProgressScale(t *testing.T) {
	got := buildFilamentUsageTemplate("sensor.test_print_weight", "sensor.test_print_progress")
	for _, want := range []string{
		"sensor.test_print_weight",
		"sensor.test_print_progress",
		"if (states('sensor.test_print_progress') | float(0) > 1) else",
		"/ 100.0",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("filament usage template missing %q.\ngot: %s", want, got)
		}
	}
	if strings.Contains(got, "\n") {
		t.Errorf("filament usage template must be single-line for YAML compatibility, got newlines")
	}
	if got == "0" {
		t.Error("expected template when entities provided")
	}
}

func TestGenerateConfigurationYAMLNoBareJinjaLines(t *testing.T) {
	printer := Printer{
		Prefix:              "03919c461204338",
		PrintWeightEntity:   "sensor.bambu_print_weight",
		PrintProgressEntity: "sensor.bambu_print_progress",
	}
	yaml := generateConfigurationYAML(printer.Prefix, CollectTrayInfos(printer), printer, "http://192.168.1.66:5000/api/webhook")
	for i, line := range strings.Split(yaml, "\n") {
		if strings.HasPrefix(line, "{%") || strings.HasPrefix(line, "{{") {
			t.Errorf("line %d has unindented Jinja and would break HA YAML: %q", i+1, line)
		}
	}
}

func TestGenerateHAPackageContainsUtilityMeter(t *testing.T) {
	printer := Printer{
		Prefix:              "03919c461204338",
		Name:                "Test A1",
		EntityID:            "sensor.bambu_print_status",
		PrintWeightEntity:   "sensor.bambu_print_weight",
		PrintProgressEntity: "sensor.bambu_print_progress",
		AMSUnits: []AMS{{
			AMSNumber: 1,
			Trays: []Tray{{
				EntityID:   "sensor.bambu_lab_a1_ams_tray_1",
				TrayNumber: 1,
				AMSNumber:  1,
			}},
		}},
	}
	yaml := GenerateHAPackage(printer, "http://192.168.1.66:5000/api/webhook")

	for _, want := range []string{
		"utility_meter:",
		"filabridge_03919c461204338_filament_usage_meter:",
		"source: sensor.filabridge_03919c461204338_filament_usage",
		"input_number:",
		"rest_command:",
		"template:",
		"automation:",
		"utility_meter.calibrate",
	} {
		if !strings.Contains(yaml, want) {
			t.Errorf("HA package missing %q", want)
		}
	}
	if strings.Contains(yaml, "cycle:") {
		t.Error("HA package must not set utility_meter cycle (cycle: none breaks HA loading)")
	}
}

func TestGenerateAutomationsPrintEndUsesPrintStatus(t *testing.T) {
	printer := Printer{
		Prefix:              "03919c461204338",
		EntityID:            "sensor.bambu_print_status",
		TaskNameEntity:      "sensor.bambu_lab_a1_task_name",
		GcodeFileEntity:     "sensor.bambu_lab_a1_gcode_file",
		PrintWeightEntity:   "sensor.bambu_print_weight",
		PrintProgressEntity: "sensor.bambu_print_progress",
	}
	yaml := generateAutomationsYAML(printer.Prefix, nil, "http://filabridge:5000/api/webhook", printer)
	for _, want := range []string{
		"entity_id: sensor.bambu_print_status",
		"id: print_start",
		"- finish",
		"trigger.from_state.state in ['running', 'pause', 'prepare', 'slicing', 'failed']",
		"states('sensor.filabridge_",
		"states('sensor.bambu_lab_a1_task_name')",
		"states('sensor.bambu_lab_a1_gcode_file')",
	} {
		if !strings.Contains(yaml, want) {
			t.Errorf("generated automations missing %q", want)
		}
	}
	if strings.Contains(yaml, "state_attr(") && strings.Contains(yaml, "subtask_name") {
		t.Error("generated automations should not read subtask_name from print_status attributes")
	}
}

func TestBuildJobNameTemplateUsesTaskNameSensor(t *testing.T) {
	got := buildJobNameTemplate("sensor.bambu_lab_a1_task_name", "sensor.bambu_lab_a1_gcode_file")
	for _, want := range []string{
		"states('sensor.bambu_lab_a1_task_name')",
		"states('sensor.bambu_lab_a1_gcode_file')",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("job name template missing %q.\ngot: %s", want, got)
		}
	}
}
