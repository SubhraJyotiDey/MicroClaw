package audio

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"
	"time"
)

// TestMultilingualTTS verifies that the Edge-TTS client successfully connects,
// sends SSML, and streams back active audio frames for all three target languages.
func TestMultilingualTTS(t *testing.T) {
	tts := NewTTSClient()

	testCases := []struct {
		name string
		text string
		lang string
	}{
		{"English", "Hello, system initializing. Check instruments.", "en"},
		{"Hindi", "नमस्ते जार्विस, उपकरण सेटअप की जाँच करें।", "hi"},
		{"Bengali", "হ্যালো জার্ভিস, বিএসসি ল্যাবরেটরি চালু করো।", "bn"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Using 8 seconds timeout since network calls are involved
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()

			buf := &bytes.Buffer{}
			err := tts.StreamSpeech(ctx, tc.text, tc.lang, "female", buf)
			if err != nil {
				t.Fatalf("Speech synthesis failed for %s: %v", tc.name, err)
			}

			if buf.Len() == 0 {
				t.Errorf("Buffer length for %s is 0, no audio payload received", tc.name)
			}

			log.Printf("[TTS Test] Language %s synthesized successfully. Payload: %d bytes.", tc.name, buf.Len())
		})
	}
}

// TestMultilingualSTT_Mock checks that the Whisper interface responds correctly
// with localized fallback text when operating in mock mode.
func TestMultilingualSTT_Mock(t *testing.T) {
	stt := NewSTTClient("gsk_mock_key", "")

	testCases := []struct {
		name     string
		lang     string
		expected string
	}{
		{"English", "en", "kettle"},
		{"Hindi", "hi", "इलेक्ट्रिक केतली"},
		{"Bengali", "bn", "জল দেওয়ার ব্যবস্থা"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dummyWav := bytes.NewReader([]byte("RIFFxxxxWAVEfmt xxxxdataxxxx"))
			result, err := stt.Transcribe(context.Background(), dummyWav, tc.lang)
			if err != nil {
				t.Fatalf("Transcribe failed: %v", err)
			}

			if !strings.Contains(strings.ToLower(result), strings.ToLower(tc.expected)) {
				t.Errorf("Expected transcript %q to contain %q", result, tc.expected)
			}
		})
	}
}
