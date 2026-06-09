package main

import (
	"strings"
	"testing"
)

func TestBuildTrayEntityLookupJinjaSyntax(t *testing.T) {
	trays := []BambuTrayInfo{
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

func TestGenerateBambuAutomationsTrayChangeAttributeTriggers(t *testing.T) {
	printer := BambuPrinter{
		Prefix: "03919c461204338",
		Name:   "Test",
		AMSUnits: []BambuAMS{{
			AMSNumber: 1,
			Trays: []BambuTray{{
				EntityID:   "sensor.bambu_lab_a1_ams_tray_1",
				TrayNumber: 1,
				AMSNumber:  1,
			}},
		}},
	}
	yaml := generateBambuAutomationsYAML(printer.Prefix, CollectBambuTrayInfos(printer), "http://filabridge:5000/api/webhook", printer)

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
