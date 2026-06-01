package agents

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// CoderAgent manages local Python script operations in a designated sandboxed environment.
type CoderAgent struct {
	sandboxDir string
}

// NewCoderAgent constructs a new CoderAgent sandbox wrapper.
func NewCoderAgent(sandboxDir string) (*CoderAgent, error) {
	if err := os.MkdirAll(sandboxDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to configure sandbox directory: %w", err)
	}
	return &CoderAgent{sandboxDir: sandboxDir}, nil
}

// WriteScript writes Python source code to a designated file inside the sandbox.
func (c *CoderAgent) WriteScript(filename string, code string) (string, error) {
	targetPath := filepath.Join(c.sandboxDir, filename)
	if err := os.WriteFile(targetPath, []byte(code), 0644); err != nil {
		return "", fmt.Errorf("failed to save script file: %w", err)
	}
	return targetPath, nil
}

// BackupScript creates a copy of the script file (.bak extension).
func (c *CoderAgent) BackupScript(filename string) error {
	srcPath := filepath.Join(c.sandboxDir, filename)
	destPath := filepath.Join(c.sandboxDir, filename+".bak")

	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("failed to read script for backup: %w", err)
	}

	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write backup script: %w", err)
	}
	return nil
}

// RestoreBackup overwrites the script file with its backup version.
func (c *CoderAgent) RestoreBackup(filename string) error {
	srcPath := filepath.Join(c.sandboxDir, filename+".bak")
	destPath := filepath.Join(c.sandboxDir, filename)

	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("failed to read backup script: %w", err)
	}

	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write restored script: %w", err)
	}
	return nil
}

// ExecuteScript runs the Python file using shell commands, capturing stdout and stderr.
// Runs under context-based timeouts (5 seconds) to prevent infinite loops.
func (c *CoderAgent) ExecuteScript(ctx context.Context, filename string) (string, string, error) {
	scriptPath := filepath.Join(c.sandboxDir, filename)

	// Resolve the correct python executable name
	pyCmd := "python"
	if runtime.GOOS != "windows" {
		if _, err := exec.LookPath("python3"); err == nil {
			pyCmd = "python3"
		}
	}

	cmdCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, pyCmd, scriptPath)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()

	if cmdCtx.Err() == context.DeadlineExceeded {
		return stdout, stderr, fmt.Errorf("script execution timed out (limit 5s)")
	}

	return stdout, stderr, err
}
