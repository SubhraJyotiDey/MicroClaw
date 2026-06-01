package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"jarvis-core/agents"
)

func TestHardwareAgent(t *testing.T) {
	dbFile := "test_hardware.db"
	defer os.Remove(dbFile)

	ma, err := agents.NewMemoryAgent(dbFile)
	if err != nil {
		t.Fatalf("MemoryAgent creation failed: %v", err)
	}
	defer ma.Close()

	// Initialize hardware agent with mock IPs
	ha := agents.NewHardwareAgent("127.0.0.1", "127.0.0.1", "", ma)

	// Test control device
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = ha.ControlDevice(ctx, "kettle", "on")
	if err != nil {
		t.Errorf("ControlDevice failed: %v", err)
	}

	// Verify logged state in SQLite
	logs, err := ma.GetRecentSensorLogs("kettle_state", 5)
	if err != nil || len(logs) != 1 || logs[0].Value != 1.0 {
		t.Errorf("Kettle state log invalid: logs=%+v, err=%v", logs, err)
	}

	// Test channel loop routing
	cmdChan := make(chan agents.HardwareCommand, 2)
	ctxLoop, cancelLoop := context.WithCancel(context.Background())
	
	go ha.StartLoop(ctxLoop, cmdChan)

	// Send kettle off
	cmdChan <- agents.HardwareCommand{Device: "kettle", Action: "off"}
	time.Sleep(100 * time.Millisecond) // Let goroutine process

	// Verify logged state
	logs, err = ma.GetRecentSensorLogs("kettle_state", 5)
	if err != nil || len(logs) < 2 {
		t.Errorf("Kettle off channel trigger failed: logs=%+v, err=%v", logs, err)
	}
	
	cancelLoop()
}

func TestVisionAgent(t *testing.T) {
	tempFile := "test_vision.jpg"
	defer os.Remove(tempFile)

	va := agents.NewVisionAgent("gsk_mock_key", "")

	// Test capture frame
	err := va.CaptureFrame(context.Background(), tempFile)
	if err != nil {
		t.Fatalf("CaptureFrame failed: %v", err)
	}

	// Assert file exists and is not empty
	info, err := os.Stat(tempFile)
	if err != nil || info.Size() == 0 {
		t.Errorf("Captured JPEG is missing or empty")
	}

	// Test analyze mock
	desc, err := va.AnalyzeFrame(context.Background(), tempFile, "Describe the ESP32 circuit board state.")
	if err != nil {
		t.Fatalf("AnalyzeFrame failed: %v", err)
	}

	if !strings.Contains(desc, "Mock") {
		t.Errorf("Expected mock response description, got: %q", desc)
	}
}

func TestPulseAgent(t *testing.T) {
	dbFile := "test_pulse.db"
	defer os.Remove(dbFile)

	ma, err := agents.NewMemoryAgent(dbFile)
	if err != nil {
		t.Fatalf("MemoryAgent creation failed: %v", err)
	}
	defer ma.Close()

	pa := agents.NewPulseAgent(ma)

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	// Start tickers in background (run every 100ms for fast testing)
	go pa.StartLoops(ctx, 100*time.Millisecond)

	time.Sleep(220 * time.Millisecond) // Let it tick twice

	// Check if sensor values got written to database
	logs, err := ma.GetRecentSensorLogs("soil_moisture", 5)
	if err != nil || len(logs) == 0 {
		t.Errorf("PulseAgent failed to log soil moisture, logs=%+v, err=%v", logs, err)
	}
}
