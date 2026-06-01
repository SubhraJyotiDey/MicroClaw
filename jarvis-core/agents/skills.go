package agents

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// SkillsManager coordinates execution of agentic tools.
type SkillsManager struct {
	healer *HealerAgent
	vision *VisionAgent
}

// NewSkillsManager constructs a new SkillsManager.
func NewSkillsManager(healer *HealerAgent, vision *VisionAgent) *SkillsManager {
	return &SkillsManager{
		healer: healer,
		vision: vision,
	}
}

// Execute triggers the named skill and returns its console string output.
func (s *SkillsManager) Execute(ctx context.Context, skillName string, skillArgs string) (string, error) {
	log.Printf("[Skills] Executing skill %s with args: %q", skillName, skillArgs)
	switch strings.ToLower(skillName) {
	case "system_info":
		return s.runSystemInfo()
	case "execute_python":
		return s.runExecutePython(ctx, skillArgs)
	case "capture_vision":
		return s.runCaptureVision(ctx, skillArgs)
	case "web_search":
		return s.runWebSearch(ctx, skillArgs)
	default:
		return "", fmt.Errorf("unknown skill: %s", skillName)
	}
}

func (s *SkillsManager) runSystemInfo() (string, error) {
	var sb strings.Builder
	sb.WriteString("=== System Statistics ===\n")
	sb.WriteString(fmt.Sprintf("OS: %s / Arch: %s\n", runtime.GOOS, runtime.GOARCH))

	// RPi CPU Temperature reading (Linux thermal zone)
	if runtime.GOOS == "linux" {
		tempBytes, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
		if err == nil {
			tempStr := strings.TrimSpace(string(tempBytes))
			if len(tempStr) > 0 {
				var tempFloat float64
				if _, scanErr := fmt.Sscanf(tempStr, "%f", &tempFloat); scanErr == nil {
					sb.WriteString(fmt.Sprintf("CPU Temperature: %.1f°C\n", tempFloat/1000.0))
				}
			}
		} else {
			sb.WriteString("CPU Temperature: Unknown (could not read thermal zone)\n")
		}

		// RPi Disk space
		cmd := exec.Command("df", "-h", "/")
		out, err := cmd.Output()
		if err == nil {
			lines := strings.Split(string(out), "\n")
			if len(lines) > 1 {
				sb.WriteString(fmt.Sprintf("Disk Usage: %s\n", strings.TrimSpace(lines[1])))
			}
		}
	} else {
		// Mock stats on non-Linux systems
		sb.WriteString("CPU Temperature: 42.5°C (Simulated RPi temperature)\n")
		sb.WriteString("Disk Usage: /dev/root  32G  4.2G   26G  14% /\n")
	}

	return sb.String(), nil
}

func (s *SkillsManager) runExecutePython(ctx context.Context, code string) (string, error) {
	filename := "execution_script.py"
	log.Printf("[Skills] Running Python code via HealerAgent...")

	// Clean out any markdown wraps if returned in arguments
	code = strings.TrimPrefix(code, "```python")
	code = strings.TrimPrefix(code, "```")
	code = strings.TrimSuffix(code, "```")
	code = strings.TrimSpace(code) + "\n"

	stdout, err := s.healer.HealAndExecute(ctx, filename, code)
	if err != nil {
		return "", fmt.Errorf("python execution failed: %w", err)
	}

	return fmt.Sprintf("=== Python Execution Output ===\n%s", stdout), nil
}

func (s *SkillsManager) runCaptureVision(ctx context.Context, prompt string) (string, error) {
	if prompt == "" {
		prompt = "Describe the workbench state."
	}

	// Use a temp file to prevent race conditions between concurrent vision calls
	tmpFile, err := os.CreateTemp("", "microclaw_vision_*.jpg")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file for capture: %w", err)
	}
	outputPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(outputPath)

	log.Printf("[Skills] Capturing camera frame to %s...", outputPath)

	err = s.vision.CaptureFrame(ctx, outputPath)
	if err != nil {
		return "", fmt.Errorf("camera capture failed: %w", err)
	}

	log.Printf("[Skills] Submitting photo to Multimodal Vision model...")
	analysis, err := s.vision.AnalyzeFrame(ctx, outputPath, prompt)
	if err != nil {
		return "", fmt.Errorf("vision analysis failed: %w", err)
	}

	return fmt.Sprintf("=== Vision Analysis ===\nPrompt: %s\nResult: %s", prompt, analysis), nil
}

func (s *SkillsManager) runWebSearch(ctx context.Context, query string) (string, error) {
	if query == "" {
		return "", fmt.Errorf("search query cannot be empty")
	}

	log.Printf("[Skills] Scraping web search results for query: %s", query)

	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("search returned status: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	body := string(bodyBytes)
	var results []string

	offset := 0
	count := 0
	for count < 3 {
		snippetIndex := strings.Index(body[offset:], "class=\"result__snippet\"")
		if snippetIndex == -1 {
			break
		}

		startIndex := offset + snippetIndex + strings.Index(body[offset+snippetIndex:], ">") + 1
		endIndex := startIndex + strings.Index(body[startIndex:], "</a>")
		if endIndex < startIndex {
			break
		}

		snippet := body[startIndex:endIndex]
		snippet = strings.ReplaceAll(snippet, "<b>", "")
		snippet = strings.ReplaceAll(snippet, "</b>", "")
		snippet = strings.TrimSpace(snippet)

		results = append(results, fmt.Sprintf("%d. %s", count+1, snippet))

		offset = endIndex
		count++
	}

	if len(results) == 0 {
		return "No search results found.", nil
	}

	return fmt.Sprintf("=== Web Search Results for %q ===\n%s", query, strings.Join(results, "\n\n")), nil
}
