package audio

import (
	"context"
	"io"
	"log"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

// Player manages audio playback through native Linux ALSA tools, ensuring
// exclusive access to the hardware soundcard via a sync.Mutex.
type Player struct {
	mu         sync.Mutex
	isSpeaking func(bool) // Callback to update global state (Blackboard)
	activeCmd  *exec.Cmd
	cmdMu      sync.Mutex
}

// NewPlayer constructs a new Player instance.
func NewPlayer(isSpeakingCallback func(bool)) *Player {
	return &Player{
		isSpeaking: isSpeakingCallback,
	}
}

// Interrupt cancels any active aplay process execution immediately.
func (p *Player) Interrupt() {
	p.cmdMu.Lock()
	defer p.cmdMu.Unlock()
	if p.activeCmd != nil && p.activeCmd.Process != nil {
		log.Println("[Player] Interrupting active playback process...")
		_ = p.activeCmd.Process.Kill()
	}
}

// PlayFiller plays a pre-cached local audio file (e.g., "Hmm") to mask latency.
// Mutex protection ensures it releases the ALSA device before any streaming TTS begins.
func (p *Player) PlayFiller(ctx context.Context, filepath string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.isSpeaking(true)
	defer p.isSpeaking(false)

	log.Printf("[Player] Playing filler audio from: %s", filepath)

	if runtime.GOOS == "windows" {
		log.Printf("[Player] [Mock] Playing filler file: %s (duration 800ms)", filepath)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(800 * time.Millisecond):
			return nil
		}
	}

	cmd := exec.CommandContext(ctx, "aplay", filepath)
	p.cmdMu.Lock()
	p.activeCmd = cmd
	p.cmdMu.Unlock()

	if err := cmd.Start(); err != nil {
		log.Printf("[Player] Warning: Failed to run aplay: %v. Mocking 800ms playback instead.", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(800 * time.Millisecond):
			return nil
		}
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		p.Interrupt()
		return ctx.Err()
	case err := <-done:
		p.cmdMu.Lock()
		p.activeCmd = nil
		p.cmdMu.Unlock()
		if err != nil && err.Error() != "signal: killed" {
			return err
		}
		return nil
	}
}

// PlayStream reads from the audioReader and streams audio playback.
func (p *Player) PlayStream(ctx context.Context, audioReader io.Reader) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.isSpeaking(true)
	defer p.isSpeaking(false)

	log.Println("[Player] Starting audio stream playback...")

	if runtime.GOOS == "windows" {
		return p.mockPlayback(ctx, audioReader)
	}

	// Assuming aplay reads raw or wav stream. If the TTS returns wav/mp3, aplay will read stdin.
	cmd := exec.CommandContext(ctx, "aplay")
	cmd.Stdin = audioReader
	p.cmdMu.Lock()
	p.activeCmd = cmd
	p.cmdMu.Unlock()

	if err := cmd.Start(); err != nil {
		log.Printf("[Player] Warning: Failed to run aplay: %v. Falling back to mock streaming.", err)
		return p.mockPlayback(ctx, audioReader)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		p.Interrupt()
		return ctx.Err()
	case err := <-done:
		p.cmdMu.Lock()
		p.activeCmd = nil
		p.cmdMu.Unlock()
		if err != nil && err.Error() != "signal: killed" {
			return err
		}
		return nil
	}
}

// mockPlayback simulates audio stream playback timing when hardware driver or command is missing.
func (p *Player) mockPlayback(ctx context.Context, r io.Reader) error {
	log.Println("[Player] [Mock] Streaming audio playback...")
	buf := make([]byte, 1024)
	for {
		select {
		case <-ctx.Done():
			log.Println("[Player] [Mock] Playback stream cancelled")
			return ctx.Err()
		default:
			n, err := r.Read(buf)
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			// Simulate playback speed: 32000 bytes/sec for 16kHz 16-bit mono PCM.
			// 1024 bytes takes ~32ms.
			time.Sleep(time.Duration(n) * time.Second / 32000)
		}
	}
}
