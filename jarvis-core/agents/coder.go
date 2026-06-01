package agents

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// CoderAgent manages local Python script operations in a designated sandboxed environment.
type CoderAgent struct {
	sandboxDir string
}

// NewCoderAgent constructs a new CoderAgent sandbox wrapper.
// Pre-warms the Python interpreter to reduce first-execution latency (~800ms on RPi Zero 2W).
func NewCoderAgent(sandboxDir string) (*CoderAgent, error) {
	if err := os.MkdirAll(sandboxDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to configure sandbox directory: %w", err)
	}

	// Pre-warm Python interpreter in background to avoid cold-start latency
	go func() {
		pyCmd := "python3"
		if runtime.GOOS == "windows" {
			pyCmd = "python"
		}
		if err := exec.Command(pyCmd, "-c", "pass").Run(); err != nil {
			log.Printf("[CoderAgent] Python pre-warm failed (non-fatal): %v", err)
		} else {
			log.Println("[CoderAgent] Python interpreter pre-warmed successfully.")
		}
	}()

	return &CoderAgent{sandboxDir: sandboxDir}, nil
}

// resolvePath validates that the target filename resides strictly inside the sandbox.
func (c *CoderAgent) resolvePath(filename string) (string, error) {
	cleanSandbox, err := filepath.Abs(c.sandboxDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve sandbox directory: %w", err)
	}

	targetPath := filepath.Clean(filepath.Join(c.sandboxDir, filename))
	targetAbs, err := filepath.Abs(targetPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve target path: %w", err)
	}

	// Verify that target path is a child of the sandbox directory
	if !strings.HasPrefix(targetAbs, cleanSandbox) {
		return "", fmt.Errorf("security violation: path traversal detected (target: %s, sandbox: %s)", targetAbs, cleanSandbox)
	}

	return targetAbs, nil
}

// WriteScript writes Python source code to a designated file inside the sandbox.
func (c *CoderAgent) WriteScript(filename string, code string) (string, error) {
	targetPath, err := c.resolvePath(filename)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(targetPath, []byte(code), 0644); err != nil {
		return "", fmt.Errorf("failed to save script file: %w", err)
	}
	return targetPath, nil
}

// BackupScript creates a copy of the script file (.bak extension).
func (c *CoderAgent) BackupScript(filename string) error {
	srcPath, err := c.resolvePath(filename)
	if err != nil {
		return err
	}
	destPath, err := c.resolvePath(filename + ".bak")
	if err != nil {
		return err
	}

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
	srcPath, err := c.resolvePath(filename + ".bak")
	if err != nil {
		return err
	}
	destPath, err := c.resolvePath(filename)
	if err != nil {
		return err
	}

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
// Runs under context-based timeouts (5 seconds or parent deadline, whichever is shorter) to prevent infinite loops.
// On Linux, the script runs as the 'nobody' user (UID 65534) for OS-level process isolation.
func (c *CoderAgent) ExecuteScript(ctx context.Context, filename string) (string, string, error) {
	scriptPath, err := c.resolvePath(filename)
	if err != nil {
		return "", "", err
	}

	// Resolve the correct python executable name
	pyCmd := "python"
	if runtime.GOOS != "windows" {
		if _, err := exec.LookPath("python3"); err == nil {
			pyCmd = "python3"
		}
	}

	timeout := 5 * time.Second
	if dl, ok := ctx.Deadline(); ok {
		remaining := time.Until(dl)
		if remaining < timeout {
			timeout = remaining
		}
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, pyCmd, scriptPath)

	// Configure platform-specific process attributes (e.g. running as nobody user on Linux)
	prepareCmdAttrs(cmd)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err = cmd.Run()
	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()

	if cmdCtx.Err() == context.DeadlineExceeded {
		return stdout, stderr, fmt.Errorf("script execution timed out (limit %v)", timeout)
	}

	return stdout, stderr, err
}
