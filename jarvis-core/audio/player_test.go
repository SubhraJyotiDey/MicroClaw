package audio

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"
)

// TestPlayerMutexLock asserts that sequential blocking prevents overlap on the audio sink.
func TestPlayerMutexLock(t *testing.T) {
	var states []bool
	var mu sync.Mutex
	isSpeaking := func(speaking bool) {
		mu.Lock()
		states = append(states, speaking)
		mu.Unlock()
	}

	p := NewPlayer(isSpeaking)

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	// 1 second mock stream payload (32KB at 32KB/sec rate)
	data1 := make([]byte, 32*1024)
	data2 := make([]byte, 32*1024)

	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_ = p.PlayStream(ctx1, bytes.NewReader(data1))
	}()

	go func() {
		defer wg.Done()
		// Stagger start to let goroutine 1 claim lock
		time.Sleep(50 * time.Millisecond)
		_ = p.PlayStream(ctx2, bytes.NewReader(data2))
	}()

	wg.Wait()
	duration := time.Since(start)

	// Since they both play for 1s sequentially, duration must be >= 1.9s.
	if duration < 1900*time.Millisecond {
		t.Errorf("Expected sequential playback lock to take >= 1.9s, took %v", duration)
	}
}

// TestPlayerInterruption asserts that context cancel stops mock audio playback immediately.
func TestPlayerInterruption(t *testing.T) {
	isSpeaking := func(speaking bool) {}
	p := NewPlayer(isSpeaking)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 3 seconds mock stream payload (96KB)
	data := make([]byte, 96*1024)

	start := time.Now()
	errChan := make(chan error, 1)

	go func() {
		errChan <- p.PlayStream(ctx, bytes.NewReader(data))
	}()

	// Wait 200ms and cancel the context, simulating VAD interrupt channel handler action
	time.Sleep(200 * time.Millisecond)
	cancel()
	p.Interrupt()

	err := <-errChan
	duration := time.Since(start)

	if err == nil {
		t.Error("Expected context cancellation error, got nil")
	}

	// Playback should stop immediately around 200-300ms, definitely well under 1 second.
	if duration > 800*time.Millisecond {
		t.Errorf("Expected interruption to break playback in < 800ms, took %v", duration)
	}
}
