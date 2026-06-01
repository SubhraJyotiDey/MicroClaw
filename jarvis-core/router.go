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
	"time"

	"jarvis-core/agents"
)

// LLMResponse is the parsed response format returned by the router to the audio synthesizer.
type LLMResponse struct {
	LanguageCode string `json:"language_code"`
	Text         string `json:"text"`
	SkillName    string `json:"skill_name,omitempty"`
	SkillArgs    string `json:"skill_args,omitempty"`
}

// Router checks triggers, executes fast path local bypassing, and schedules LLM completions.
type Router struct {
	hardwareChan chan<- agents.HardwareCommand
	apiKey       string
	apiURL       string
	systemPrompt string
}

// NewRouter constructs a new intent router.
func NewRouter(hardwareChan chan<- agents.HardwareCommand, apiKey, apiURL, systemPrompt string) *Router {
	return &Router{
		hardwareChan: hardwareChan,
		apiKey:       apiKey,
		apiURL:       apiURL,
		systemPrompt: systemPrompt,
	}
}

func containsAny(text string, keywords ...string) bool {
	t := strings.ToLower(text)
	for _, kw := range keywords {
		if strings.Contains(t, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// Route executes keyword checks for immediate response or delegates execution to Groq/OpenRouter.
func (r *Router) Route(ctx context.Context, userText string, cm *ContextManager) (*LLMResponse, bool, error) {
	cleanedText := strings.TrimSpace(userText)

	hasKettle := containsAny(cleanedText, "kettle", "केतली", "কেতলি")
	hasIrrig  := containsAny(cleanedText, "irrigation", "water", "plants", "पानी", "पौधों", "জল", "গাছ", "সেচ")
	hasOn     := containsAny(cleanedText, "on", "चालू", "চালু")
	hasOff    := containsAny(cleanedText, "off", "बंद", "বন্ধ")

	// Check Kettle Fast-Path
	if hasKettle && (hasOn || hasOff) {
		action := "on"
		if hasOff {
			action = "off"
		}
		log.Printf("[Router] Fast-Path Bypass: Device=kettle Action=%s", action)
		r.sendHardwareCmd(agents.HardwareCommand{Device: "kettle", Action: action})
		return r.getFastResponse(cleanedText, "kettle", action), true, nil
	}

	// Check Irrigation Fast-Path
	if hasIrrig && (hasOn || hasOff) {
		action := "on"
		if hasOff {
			action = "off"
		}
		log.Printf("[Router] Fast-Path Bypass: Device=irrigation Action=%s", action)
		r.sendHardwareCmd(agents.HardwareCommand{Device: "irrigation", Action: action})
		return r.getFastResponse(cleanedText, "irrigation", action), true, nil
	}

	// Slow-Path: Query Brain LLM
	log.Println("[Router] Fast-path mismatch. Querying LLM...")
	llmResp, err := r.callLLM(ctx, cm.GetMessagesForLLM(r.systemPrompt))
	if err != nil {
		return nil, false, err
	}

	return llmResp, false, nil
}

func (r *Router) sendHardwareCmd(cmd agents.HardwareCommand) {
	select {
	case r.hardwareChan <- cmd:
		log.Printf("[Router] Hardware command routed to channel: %+v", cmd)
	default:
		log.Println("[Router] Warning: Hardware channel full, command dropped.")
	}
}

func matchesOff(text string) bool {
	t := strings.ToLower(text)
	return strings.Contains(t, "off") || strings.Contains(t, "बंद") || strings.Contains(t, "বন্ধ")
}

func detectLang(text string) string {
	// Check for Devanagari range (Hindi)
	for _, c := range text {
		if c >= 0x0900 && c <= 0x097F {
			return "hi"
		}
		// Check for Bengali range
		if c >= 0x0980 && c <= 0x09FF {
			return "bn"
		}
	}
	return "en"
}

func (r *Router) getFastResponse(userText, device, action string) *LLMResponse {
	lang := detectLang(userText)
	var text string

	switch lang {
	case "hi":
		if device == "kettle" {
			if action == "on" {
				text = "ठीक है, मैं इलेक्ट्रिक केतली चालू कर रहा हूँ।"
			} else {
				text = "ठीक है, मैं इलेक्ट्रिक केतली बंद कर रहा हूँ।"
			}
		} else {
			if action == "on" {
				text = "पौधों में पानी डालने का सिस्टम चालू किया जा रहा है।"
			} else {
				text = "पौधों में पानी डालने का सिस्टम बंद किया जा रहा है।"
			}
		}
	case "bn":
		if device == "kettle" {
			if action == "on" {
				text = "ঠিক আছে, আমি ইলেকট্রিক কেতলিটি চালু করছি।"
			} else {
				text = "ঠিক আছে, আমি ইলেকট্রিক কেতলিটি বন্ধ করছি।"
			}
		} else {
			if action == "on" {
				text = "গাছপালায় জল দেওয়ার ব্যবস্থা চালু করা হচ্ছে।"
			} else {
				text = "গাছপালায় জল দেওয়ার ব্যবস্থা বন্ধ করা হচ্ছে।"
			}
		}
	default: // English fallback
		if device == "kettle" {
			if action == "on" {
				text = "Understood. Switching the electric kettle on."
			} else {
				text = "Understood. Switching the electric kettle off."
			}
		} else {
			if action == "on" {
				text = "Processing. Initiating balcony irrigation pumps."
			} else {
				text = "Processing. Halting balcony irrigation pumps."
			}
		}
	}

	return &LLMResponse{
		LanguageCode: lang,
		Text:         text,
	}
}

func (r *Router) callLLM(ctx context.Context, messages []Message) (*LLMResponse, error) {
	if r.apiKey == "" || strings.Contains(r.apiKey, "mock") {
		// Mock response if API keys are empty/unset
		return &LLMResponse{LanguageCode: "en", Text: "Mock Response: I am assisting with lab instrumentation calculations."}, nil
	}

	// Determine endpoint and model choice
	model := "llama-3.3-70b-versatile" // default Groq
	if strings.Contains(r.apiURL, "openrouter.ai") {
		model = "google/gemini-2.5-flash"
	}

	payload := map[string]interface{}{
		"model":       model,
		"messages":    messages,
		"temperature": 0.3,
	}

	// Instruct LLM to output JSON format
	// Both Groq and OpenRouter support response_format json_object
	payload["response_format"] = map[string]string{"type": "json_object"}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", r.apiURL, bytes.NewReader(jsonPayload))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.apiKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("brain llm status %d: %s", resp.StatusCode, string(respBytes))
	}

	var apiResult struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResult); err != nil {
		return nil, err
	}

	if len(apiResult.Choices) == 0 {
		return nil, fmt.Errorf("no choice content returned by LLM")
	}

	content := strings.TrimSpace(apiResult.Choices[0].Message.Content)

	// Clean markdown wrappers if returned
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var llmResp LLMResponse
	if err := json.Unmarshal([]byte(content), &llmResp); err != nil {
		log.Printf("[Router] Warning: LLM output was not valid JSON or mismatch format: %q. Attempting text fallback.", content)
		// Fallback parse if LLM did not return strict JSON despite formats
		return &LLMResponse{
			LanguageCode: "en",
			Text:         content,
		}, nil
	}

	return &llmResp, nil
}
