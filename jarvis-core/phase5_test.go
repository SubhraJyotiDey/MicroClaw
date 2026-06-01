package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"jarvis-core/agents"
)

func TestCoderAgent(t *testing.T) {
	sandbox := "test_sandbox"
	defer os.RemoveAll(sandbox)

	coder, err := agents.NewCoderAgent(sandbox)
	if err != nil {
		t.Fatalf("Failed to create CoderAgent: %v", err)
	}

	filename := "test_script.py"
	code := "print('Hello from Sandbox!')\n"

	// Test WriteScript
	path, err := coder.WriteScript(filename, code)
	if err != nil {
		t.Fatalf("WriteScript failed: %v", err)
	}

	// Assert file exists
	if _, err := os.Stat(path); err != nil {
		t.Errorf("Written script path does not exist: %s", path)
	}

	// Test ExecuteScript
	stdout, stderr, err := coder.ExecuteScript(context.Background(), filename)
	if err != nil {
		t.Fatalf("ExecuteScript failed: %v, stderr=%s", err, stderr)
	}

	if strings.TrimSpace(stdout) != "Hello from Sandbox!" {
		t.Errorf("Expected stdout 'Hello from Sandbox!', got %q", stdout)
	}

	// Test Backup and Restore
	err = coder.BackupScript(filename)
	if err != nil {
		t.Fatalf("BackupScript failed: %v", err)
	}

	// Overwrite original script with dummy text
	_, err = coder.WriteScript(filename, "dummy")
	if err != nil {
		t.Fatalf("Overwrite failed: %v", err)
	}

	// Restore backup
	err = coder.RestoreBackup(filename)
	if err != nil {
		t.Fatalf("RestoreBackup failed: %v", err)
	}

	// Execute again to confirm original code was restored
	stdout, _, err = coder.ExecuteScript(context.Background(), filename)
	if err != nil {
		t.Fatalf("ExecuteScript after restore failed: %v", err)
	}

	if strings.TrimSpace(stdout) != "Hello from Sandbox!" {
		t.Errorf("Restore failed. Expected stdout 'Hello from Sandbox!', got %q", stdout)
	}
}

func TestHealerAgent(t *testing.T) {
	sandbox := "test_healer_sandbox"
	defer os.RemoveAll(sandbox)

	coder, err := agents.NewCoderAgent(sandbox)
	if err != nil {
		t.Fatalf("Failed to create CoderAgent: %v", err)
	}

	// We pass mock key "gsk_mock_key" to healer so it uses mock repair logic.
	healer := agents.NewHealerAgent(coder, "gsk_mock_key", "")

	// 1. Test healing on a crashing script
	// Code has a syntax/name error: print(undefined_variable)
	crashedCode := "print(undefined_variable)\n"
	filename := "healing_script.py"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stdout, err := healer.HealAndExecute(ctx, filename, crashedCode)
	if err != nil {
		t.Fatalf("HealAndExecute failed: %v", err)
	}

	// The healer agent mock repair function replaces code with print('Healed successfully!')
	if strings.TrimSpace(stdout) != "Healed successfully!" {
		t.Errorf("Expected healed stdout 'Healed successfully!', got %q", stdout)
	}

	// Assert that backup exists
	bakPath := sandbox + "/" + filename + ".bak"
	if _, err := os.Stat(bakPath); err != nil {
		t.Errorf("Expected backup script to exist at: %s", bakPath)
	}
}
