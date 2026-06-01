package audio

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"strings"
	"time"
)

// STTClient wraps Whisper API configuration (like Groq API).
type STTClient struct {
	apiKey string
	apiURL string
}

// NewSTTClient initializes a new STT Whisper transcriber.
func NewSTTClient(apiKey, apiURL string) *STTClient {
	if apiKey == "" {
		apiKey = os.Getenv("GROQ_API_KEY")
	}
	if apiURL == "" {
		apiURL = os.Getenv("API_URL_STT")
		if apiURL == "" {
			apiURL = "https://api.groq.com/openai/v1/audio/transcriptions"
		}
	}
	return &STTClient{
		apiKey: apiKey,
		apiURL: apiURL,
	}
}

// Transcribe sends raw WAV audio stream payload to the Whisper API, returning transcription text.
// If the API Key is missing or matches mock pattern, it falls back to mock responses.
func (s *STTClient) Transcribe(ctx context.Context, wavReader io.Reader, lang string) (string, error) {
	if s.apiKey == "" || strings.HasPrefix(s.apiKey, "gsk_mock") {
		logMockTranscribe(lang)
		return getMockTranscript(lang), nil
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Create multipart file header
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="input.wav"`)
	h.Set("Content-Type", "audio/wav")
	part, err := writer.CreatePart(h)
	if err != nil {
		return "", fmt.Errorf("stt body partition failed: %w", err)
	}

	if _, err := io.Copy(part, wavReader); err != nil {
		return "", fmt.Errorf("stt audio buffer copy failed: %w", err)
	}

	if err := writer.WriteField("model", "whisper-large-v3-turbo"); err != nil {
		return "", fmt.Errorf("stt payload model injection failed: %w", err)
	}

	if err := writer.WriteField("response_format", "text"); err != nil {
		return "", fmt.Errorf("stt payload response_format injection failed: %w", err)
	}

	if lang != "" && lang != "auto" {
		if err := writer.WriteField("language", lang); err != nil {
			return "", fmt.Errorf("stt payload language injection failed: %w", err)
		}
		var prompt string
		if lang == "bn" {
			prompt = "গোপাল ভাঁড়, মহারাজ কৃষ্ণচন্দ্র, বাংলা হরফ, বাঙালি"
		} else if lang == "hi" {
			prompt = "गोपाल भार, महाराज कृष्णचंद्र, हिंदी वार्तालाप"
		} else if lang == "en" {
			prompt = "Gopal Bhar, King Krishnachandra, English conversation"
		}
		if prompt != "" {
			if err := writer.WriteField("prompt", prompt); err != nil {
				return "", fmt.Errorf("stt payload prompt injection failed: %w", err)
			}
		}
	} else {
		// Provide a multilingual context prompt for auto-detection
		if err := writer.WriteField("prompt", "Gopal Bhar, Gopal, গোপাল ভাঁড়, গোপাল, মহারাজ কৃষ্ণচন্দ্র, Krishnachandra"); err != nil {
			return "", fmt.Errorf("stt payload prompt injection failed: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("stt body closer failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.apiURL, body)
	if err != nil {
		return "", fmt.Errorf("stt request build failed: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("stt endpoint connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("stt api returned error status %d: %s", resp.StatusCode, string(respBytes))
	}

	// response_format=text returns raw transcript text, not JSON
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("stt response read failed: %w", err)
	}

	return strings.TrimSpace(string(respBytes)), nil
}

func logMockTranscribe(lang string) {
	println(fmt.Sprintf("[STT] [Mock] Transcribing audio with language hints: %q", lang))
}

func getMockTranscript(lang string) string {
	switch strings.ToLower(lang) {
	case "hi":
		return "नमस्ते जार्विस, इलेक्ट्रिक केतली का स्टेटस क्या है?"
	case "bn":
		return "হ্যালো জার্ভিস, গাছপালার জল দেওয়ার ব্যবস্থা কি চালু আছে?"
	default:
		return "Hello Jarvis, is the kettle turned off?"
	}
}
