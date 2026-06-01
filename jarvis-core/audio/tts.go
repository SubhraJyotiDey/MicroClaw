package audio

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/wujunwei928/edge-tts-go/edge_tts"
)

// TTSClient handles neural voice synthesis using the edge-tts-go library.
type TTSClient struct {
	femaleVoices map[string]string
	maleVoices   map[string]string
}

// NewTTSClient constructs a new Edge-TTS service wrapper.
func NewTTSClient() *TTSClient {
	return &TTSClient{
		femaleVoices: map[string]string{
			"en": "en-US-AriaNeural",
			"hi": "hi-IN-SwaraNeural",
			"bn": "bn-IN-TanishaaNeural",
		},
		maleVoices: map[string]string{
			"en": "en-US-ChristopherNeural",
			"hi": "hi-IN-MadhurNeural",
			"bn": "bn-IN-BashkarNeural",
		},
	}
}

// MapLanguageToVoice resolves the language ISO tag ('en', 'hi', 'bn') and gender to Neural Edge voice models.
func (t *TTSClient) MapLanguageToVoice(lang string, gender string) string {
	voiceMap := t.femaleVoices
	if strings.ToLower(gender) == "male" {
		voiceMap = t.maleVoices
	}
	voice, ok := voiceMap[strings.ToLower(lang)]
	if !ok {
		return voiceMap["en"] // fallback to English
	}
	return voice
}

// ChunkText splits a paragraph of text into distinct sentences based on punctuation.
// This allows streaming synthesis on a sentence-by-sentence basis to optimize latency.
func ChunkText(text string) []string {
	var chunks []string
	var current strings.Builder
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		current.WriteRune(r)
		if r == '.' || r == '!' || r == '?' || r == '।' || r == '\n' {
			trimmed := strings.TrimSpace(current.String())
			if len(trimmed) > 0 {
				chunks = append(chunks, trimmed)
			}
			current.Reset()
		}
	}
	trimmed := strings.TrimSpace(current.String())
	if len(trimmed) > 0 {
		chunks = append(chunks, trimmed)
	}
	return chunks
}

// StreamSpeech synthesizes a single text string into binary audio sent to the writer in real time.
func (t *TTSClient) StreamSpeech(ctx context.Context, text string, lang string, gender string, writer io.Writer) error {
	voice := t.MapLanguageToVoice(lang, gender)
	log.Printf("[TTS] StreamSpeech requested (voice=%s, lang=%s, gender=%s): %q", voice, lang, gender, text)

	opts := []edge_tts.CommunicateOption{
		edge_tts.SetVoice(voice),
		edge_tts.SetRate("+0%"),
		edge_tts.SetVolume("+0%"),
	}

	comm, err := edge_tts.NewCommunicate(text, opts...)
	if err != nil {
		return fmt.Errorf("failed to create edge_tts communicator: %w", err)
	}

	// Retrieve synthesized audio bytes
	audioData, err := comm.Stream()
	if err != nil {
		return fmt.Errorf("edge_tts streaming failed: %w", err)
	}

	if _, err := writer.Write(audioData); err != nil {
		return fmt.Errorf("failed to write audio data to destination: %w", err)
	}

	return nil
}
