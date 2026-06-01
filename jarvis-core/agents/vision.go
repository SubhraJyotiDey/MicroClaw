package agents

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// VisionAgent handles capturing webcamera photos and submitting them to multimodal LLMs.
type VisionAgent struct {
	apiKey string
	apiURL string
}

// NewVisionAgent initializes a new VisionAgent with API endpoints.
func NewVisionAgent(apiKey, apiURL string) *VisionAgent {
	if apiKey == "" {
		apiKey = os.Getenv("GROQ_API_KEY")
	}
	if apiURL == "" {
		apiURL = os.Getenv("API_URL_LLM")
	}
	return &VisionAgent{
		apiKey: apiKey,
		apiURL: apiURL,
	}
}

// CaptureFrame runs fswebcam to save a local frame. Works with Windows/Linux mock fallbacks.
func (v *VisionAgent) CaptureFrame(ctx context.Context, outputPath string) error {
	log.Printf("[VisionAgent] Capturing camera frame to: %s", outputPath)

	if runtime.GOOS == "windows" {
		log.Println("[VisionAgent] [Mock] Writing placeholder JPEG (Windows environment).")
		// Write dummy valid JPEG markers
		return os.WriteFile(outputPath, []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0xD9}, 0644)
	}

	cmd := exec.CommandContext(ctx, "fswebcam", "--no-banner", "-S", "5", outputPath)
	if err := cmd.Run(); err != nil {
		log.Printf("[VisionAgent] Warning: fswebcam execution failed: %v. Writing fallback placeholder.", err)
		return os.WriteFile(outputPath, []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0xD9}, 0644)
	}

	return nil
}

// AnalyzeFrame reads a local photo, converts it to base64, and calls the vision API.
func (v *VisionAgent) AnalyzeFrame(ctx context.Context, imagePath string, userPrompt string) (string, error) {
	imgBytes, err := os.ReadFile(imagePath)
	if err != nil {
		return "", fmt.Errorf("failed to open vision frame: %w", err)
	}

	imgBase64 := base64.StdEncoding.EncodeToString(imgBytes)
	imgDataURL := fmt.Sprintf("data:image/jpeg;base64,%s", imgBase64)

	if v.apiKey == "" || strings.Contains(v.apiKey, "mock") {
		return "Mock Analysis: The webcam image shows an instrumentation circuit board with a green indicator LED lit, wired to a kettle relay.", nil
	}

	// Select model and target URL
	model := "llama-3.2-11b-vision-preview"
	url := "https://api.groq.com/openai/v1/chat/completions"

	if strings.Contains(v.apiURL, "openrouter.ai") {
		model = "google/gemini-2.5-flash"
		url = v.apiURL
	}

	payload := map[string]interface{}{
		"model": model,
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": userPrompt,
					},
					{
						"type": "image_url",
						"image_url": map[string]string{
							"url": imgDataURL,
						},
					},
				},
			},
		},
		"max_tokens": 150,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonPayload))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+v.apiKey)

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("vision API returned HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("empty choice array returned by vision API")
	}

	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}
