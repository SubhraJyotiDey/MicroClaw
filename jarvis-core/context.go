package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Message is the standard OpenAI-compatible role/content struct.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ContextManager maintains the short-term sliding raw history and the compressed long-term summary.
type ContextManager struct {
	mu             sync.RWMutex
	rawHistory     []Message
	runningSummary string
}

// NewContextManager instantiates a clean memory manager.
func NewContextManager() *ContextManager {
	return &ContextManager{
		rawHistory: make([]Message, 0),
	}
}

// AddMessage logs a dialogue message to the sliding window.
func (cm *ContextManager) AddMessage(role, content string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.rawHistory = append(cm.rawHistory, Message{Role: role, Content: content})
}

// GetMessagesForLLM prepares the full list of messages to feed the brain LLM.
// Structure: [System Prompt] -> [Running Summary of old turns (system prompt)] -> [Last 4 raw messages]
func (cm *ContextManager) GetMessagesForLLM(systemPrompt string) []Message {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var msgs []Message
	// Add persona instructions
	msgs = append(msgs, Message{Role: "system", Content: systemPrompt})

	// Add summarized old logs
	if cm.runningSummary != "" {
		summaryPrompt := fmt.Sprintf("Summary of previous context: %s. Rely on this summary to answer user questions about past interactions.", cm.runningSummary)
		msgs = append(msgs, Message{Role: "system", Content: summaryPrompt})
	}

	// Add last 4 raw messages
	start := 0
	if len(cm.rawHistory) > 4 {
		start = len(cm.rawHistory) - 4
	}
	msgs = append(msgs, cm.rawHistory[start:]...)

	return msgs
}

// CompressIfNeeded compresses history exceeding the raw limit (4 turns) using a fast LLM pass.
func (cm *ContextManager) CompressIfNeeded(ctx context.Context, apiKey, apiURL string) error {
	cm.mu.Lock()
	historyLen := len(cm.rawHistory)
	cm.mu.Unlock()

	if historyLen <= 4 {
		return nil // Under the threshold, no action needed
	}

	cm.mu.Lock()
	toCompress := cm.rawHistory[:historyLen-4]
	keep := cm.rawHistory[historyLen-4:]
	currentSummary := cm.runningSummary
	cm.mu.Unlock()

	log.Printf("[Context] Pruning raw context. Compressing %d messages into summary...", len(toCompress))

	historyBytes, err := json.Marshal(toCompress)
	if err != nil {
		return err
	}

	prompt := fmt.Sprintf("Please synthesize the following recent user/assistant dialogue logs along with their current running summary into a single, cohesive, updated running summary of EXACTLY 1 to 2 sentences.\n\n"+
		"Current Summary: %s\n\nDialogue to compress: %s\n\nUpdated Summary:", currentSummary, string(historyBytes))

	newSummary, err := callCompressionLLM(ctx, prompt, apiKey, apiURL)
	if err != nil {
		return fmt.Errorf("context compression failed: %w", err)
	}

	cm.mu.Lock()
	cm.runningSummary = newSummary
	cm.rawHistory = keep
	cm.mu.Unlock()

	log.Printf("[Context] Compressing complete. New summary: %q", newSummary)
	return nil
}

func callCompressionLLM(ctx context.Context, prompt, apiKey, apiURL string) (string, error) {
	if apiKey == "" || strings.Contains(apiKey, "mock") {
		return "Mock summary: User discussed local instrumentation kettle settings.", nil
	}

	// Resolve model type
	model := "llama-3.1-8b-instant"
	if strings.Contains(apiURL, "openrouter.ai") {
		model = "google/gemini-2.5-flash"
	}

	payload := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.2,
		"max_tokens":  120,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(jsonPayload))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("compression api status %d: %s", resp.StatusCode, string(respBytes))
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
		return "", fmt.Errorf("no choice returned from compression api")
	}

	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}
