package bambu

import (
	"testing"

	"filabridge/homeassistant"
)

func TestValidateHAEntitiesMeterMissing(t *testing.T) {
	states := []homeassistant.State{
		{EntityID: "sensor.filabridge_03919c461204338_filament_usage", State: "1.56"},
	}
	result := ValidateHAEntities("03919c461204338", states)
	if result.AllOK {
		t.Fatal("expected validation failure when meter is missing")
	}
	if !result.MeterMissing {
		t.Fatal("expected meter_missing=true")
	}
	if len(result.FixSteps) == 0 {
		t.Fatal("expected fix steps when entities missing")
	}
}

func TestValidateHAEntitiesAllPresent(t *testing.T) {
	prefix := "03919c461204338"
	states := []homeassistant.State{
		{EntityID: "sensor.filabridge_" + prefix + "_filament_usage", State: "0"},
		{EntityID: "sensor.filabridge_" + prefix + "_filament_usage_meter", State: "0"},
		{EntityID: "input_number.filabridge_" + prefix + "_last_tray", State: "13"},
		{EntityID: "sensor.filabridge_" + prefix + "_active_tray", State: "13"},
	}
	result := ValidateHAEntities(prefix, states)
	if !result.AllOK {
		t.Fatalf("expected all_ok, got checks=%+v fix=%v", result.Checks, result.FixSteps)
	}
	if result.MeterMissing {
		t.Fatal("expected meter_missing=false")
	}
}
