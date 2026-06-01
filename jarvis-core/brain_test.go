package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"jarvis-core/agents"
)

// TestRouterFastPath asserts regex captures route hardware controls correctly.
func TestRouterFastPath(t *testing.T) {
	hwChan := make(chan agents.HardwareCommand, 10)
	router := NewRouter(hwChan, "gsk_mock_key", "", "System persona instructions")

	testCases := []struct {
		input          string
		expectedDevice string
		expectedAction string
		expectedLang   string
	}{
		{"Turn on the kettle please", "kettle", "on", "en"},
		{"केतली बंद करो", "kettle", "off", "hi"},
		{"গাছপালায় জল দেওয়ার ব্যবস্থা চালু করো", "irrigation", "on", "bn"},
		{"turn off irrigation", "irrigation", "off", "en"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			resp, isFast, err := router.Route(context.Background(), tc.input, NewContextManager())
			if err != nil {
				t.Fatalf("Route failed: %v", err)
			}
			if !isFast {
				t.Error("Expected fast-path match, executed slow path")
			}
			if resp.LanguageCode != tc.expectedLang {
				t.Errorf("Expected language %s, got %s", tc.expectedLang, resp.LanguageCode)
			}
			
			select {
			case cmd := <-hwChan:
				if cmd.Device != tc.expectedDevice || cmd.Action != tc.expectedAction {
					t.Errorf("Expected command %+v, got %+v", agents.HardwareCommand{Device: tc.expectedDevice, Action: tc.expectedAction}, cmd)
				}
			default:
				t.Error("Hardware command failed to write to channel")
			}
		})
	}
}

// TestContextSlidingWindow asserts that history keeps raw logs under 4 and wraps summary.
func TestContextSlidingWindow(t *testing.T) {
	cm := NewContextManager()

	cm.AddMessage("user", "Hello 1")
	cm.AddMessage("assistant", "Hi 1")
	cm.AddMessage("user", "Hello 2")
	cm.AddMessage("assistant", "Hi 2")
	cm.AddMessage("user", "Hello 3")
	cm.AddMessage("assistant", "Hi 3")

	msgs := cm.GetMessagesForLLM("System prompt")
	if len(msgs) != 5 { // 1 system + 4 raw
		t.Errorf("Expected 5 messages, got %d", len(msgs))
	}

	if msgs[1].Content != "Hello 2" || msgs[4].Content != "Hi 3" {
		t.Errorf("Invalid sliding bounds: %q and %q", msgs[1].Content, msgs[4].Content)
	}

	err := cm.CompressIfNeeded(context.Background(), "gsk_mock_key", "")
	if err != nil {
		t.Fatalf("Context compression failed: %v", err)
	}

	msgs = cm.GetMessagesForLLM("System prompt")
	if len(msgs) != 6 { // 1 system + 1 summary + 4 raw
		t.Errorf("Expected 6 messages after compression, got %d", len(msgs))
	}

	if !strings.Contains(msgs[1].Content, "Summary of previous context") {
		t.Errorf("Summary prompt header mismatch: %s", msgs[1].Content)
	}
}

// TestMemorySQLite asserts schema updates and read/write operations succeed in sqlite.
func TestMemorySQLite(t *testing.T) {
	dbFile := "test_jarvis.db"
	defer os.Remove(dbFile)

	ma, err := agents.NewMemoryAgent(dbFile)
	if err != nil {
		t.Fatalf("Failed opening memory agent: %v", err)
	}
	defer ma.Close()

	// Settings Test
	err = ma.SaveSetting("irrigation_interval", "30m")
	if err != nil {
		t.Errorf("Failed saving setting: %v", err)
	}
	val, err := ma.GetSetting("irrigation_interval")
	if err != nil || val != "30m" {
		t.Errorf("Failed getting setting: %s (%v)", val, err)
	}

	// Sensor Log Test
	err = ma.LogSensor("soil_moisture", 45.2, false)
	if err != nil {
		t.Errorf("Failed logging sensor: %v", err)
	}
	logs, err := ma.GetRecentSensorLogs("soil_moisture", 5)
	if err != nil || len(logs) != 1 || logs[0].Value != 45.2 {
		t.Errorf("Failed retrieving sensor log: %+v (%v)", logs, err)
	}

	// GATE Exam Notes Test
	err = ma.AddGateQuestion("Instrumentation", "Define LVDT", "Linear Variable Differential Transformer", "hard")
	if err != nil {
		t.Errorf("Failed saving GATE note: %v", err)
	}
	q, err := ma.GetRandomGateQuestion("Instrumentation")
	if err != nil || q == nil || q.Question != "Define LVDT" {
		t.Errorf("Failed getting GATE note: %+v (%v)", q, err)
	}
}
