package audio

import (
	"context"
	"log"
	"sync"
	"time"
)

// VAD monitors the microphone stream and triggers interruptions when sustained speech is detected.
type VAD struct {
	interruptChan chan<- struct{}
	isSpeaking    func() bool
	mu            sync.Mutex
	isMuted       bool
}

// NewVAD initializes a new Voice Activity Detection handler.
func NewVAD(interruptChan chan<- struct{}, isSpeakingCallback func() bool) *VAD {
	return &VAD{
		interruptChan: interruptChan,
		isSpeaking:    isSpeakingCallback,
	}
}

// SetMute mutes or unmutes the VAD sensor logic.
func (v *VAD) SetMute(muted bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.isMuted = muted
}

// TriggerInterrupt issues a non-blocking signal to the global interrupt channel.
func (v *VAD) TriggerInterrupt() {
	log.Println("[VAD] Sustained human speech detected. Triggering non-blocking interruption...")
	select {
	case v.interruptChan <- struct{}{}:
		log.Println("[VAD] Interrupt signal successfully routed onto channel.")
	default:
		log.Println("[VAD] Interrupt signal dropped (orchestration loop not listening / buffer full).")
	}
}

// StartLoop executes the background sensor loop simulating mic stream analysis.
// If sustained speech is detected (>300ms) and Jarvis is NOT speaking (echo loop mitigation),
// it fires the interrupt signal.
func (v *VAD) StartLoop(ctx context.Context) {
	log.Println("[VAD] Microphone monitor loop active.")
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	var speechDuration time.Duration

	for {
		select {
		case <-ctx.Done():
			log.Println("[VAD] Monitor loop terminated.")
			return
		case <-ticker.C:
			v.mu.Lock()
			muted := v.isMuted
			v.mu.Unlock()

			if muted {
				speechDuration = 0
				continue
			}

			// Acoustic Echo Loop Mitigation:
			// Ignore mic feed if ALSA output buffer is active to avoid feeding JARVIS voice back to STT.
			if v.isSpeaking() {
				speechDuration = 0
				continue
			}

			// Simulated VAD amplitude threshold trigger (disabled by default in background
			// to avoid spamming. Can be forced externally or via tests).
			if detectMockSpeech() {
				speechDuration += 50 * time.Millisecond
				if speechDuration >= 300*time.Millisecond {
					v.TriggerInterrupt()
					speechDuration = 0
				}
			} else {
				speechDuration = 0
			}
		}
	}
}

// detectMockSpeech simulates speech activity. For Phase 1 we keep it false
// and let tests trigger manual interrupts to assert concurrency safety.
func detectMockSpeech() bool {
	return false
}
