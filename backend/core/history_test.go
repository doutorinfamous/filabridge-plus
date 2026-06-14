package core

import "testing"

func TestGetPrintJobsResolvesSlotNamesFromPrinterSlots(t *testing.T) {
	bridge := newTestBridge(t)

	if err := bridge.SavePrinterConfig("printer1", PrinterConfig{
		Name:      "Snapmaker",
		IPAddress: "127.0.0.1",
		Toolheads: 2,
	}); err != nil {
		t.Fatalf("SavePrinterConfig failed: %v", err)
	}

	// Toolhead slot with a custom name; tray slot from a Bambu printer.
	if err := bridge.SetToolheadName("printer1", 0, "Hotend Esquerdo"); err != nil {
		t.Fatalf("SetToolheadName failed: %v", err)
	}
	if err := bridge.SetSlotSpool("tray_a", "bambu_x1c", SlotTypeAMSTray, "X1C - AMS 1 Slot 1", 5); err != nil {
		t.Fatalf("SetSlotSpool failed: %v", err)
	}

	jobID, err := bridge.StartPrintJob("printer1", "model.gcode")
	if err != nil {
		t.Fatalf("StartPrintJob failed: %v", err)
	}
	if err := bridge.LogToolheadUsage(jobID, "printer1", 0, 7, 12.5); err != nil {
		t.Fatalf("LogToolheadUsage failed: %v", err)
	}
	if err := bridge.LogTrayUsage(jobID, "bambu_x1c", "tray_a", 5, 3.25); err != nil {
		t.Fatalf("LogTrayUsage failed: %v", err)
	}

	jobs, total, err := bridge.GetPrintJobs("", 10, 0)
	if err != nil {
		t.Fatalf("GetPrintJobs failed: %v", err)
	}
	if total != 1 || len(jobs) != 1 {
		t.Fatalf("expected 1 job, got total=%d len=%d", total, len(jobs))
	}

	job := jobs[0]
	if len(job.Usage) != 2 {
		t.Fatalf("expected 2 usage entries, got %d (%+v)", len(job.Usage), job.Usage)
	}

	slotNames := map[string]bool{}
	for _, usage := range job.Usage {
		slotNames[usage.SlotName] = true
	}
	if !slotNames["Hotend Esquerdo"] {
		t.Fatalf("expected toolhead slot name resolved, got %v", slotNames)
	}
	if !slotNames["X1C - AMS 1 Slot 1"] {
		t.Fatalf("expected tray slot name resolved, got %v", slotNames)
	}
	if job.TotalGrams != 15.75 {
		t.Fatalf("expected 15.75g total, got %.2f", job.TotalGrams)
	}
}
