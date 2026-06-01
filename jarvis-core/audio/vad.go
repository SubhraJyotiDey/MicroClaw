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

// VAD monitors the microphone stream and triggers callbacks when human speech starts/stops.
type VAD struct {
	onSpeechRecorded func([]byte)
	isSpeaking       func() bool
	mu               sync.Mutex
	isMuted          bool
}

// NewVAD initializes a new Voice Activity Detection handler.
func NewVAD(onSpeechRecorded func([]byte), isSpeakingCallback func() bool) *VAD {
	return &VAD{
		onSpeechRecorded: onSpeechRecorded,
		isSpeaking:       isSpeakingCallback,
	}
}

// SetMute mutes or unmutes the VAD sensor logic.
func (v *VAD) SetMute(muted bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.isMuted = muted
}

// ringBuffer is a FIFO buffer of fixed size chunks to hold audio pre-roll.
type ringBuffer struct {
	chunks [][]byte
	max    int
}

func newRingBuffer(max int) *ringBuffer {
	return &ringBuffer{
		chunks: make([][]byte, 0, max),
		max:    max,
	}
}

func (r *ringBuffer) Add(chunk []byte) {
	if len(r.chunks) >= r.max {
		r.chunks = r.chunks[1:]
	}
	cp := make([]byte, len(chunk))
	copy(cp, chunk)
	r.chunks = append(r.chunks, cp)
}

func (r *ringBuffer) Flatten() []byte {
	var totalLen int
	for _, c := range r.chunks {
		totalLen += len(c)
	}
	res := make([]byte, totalLen)
	var pos int
	for _, c := range r.chunks {
		copy(res[pos:], c)
		pos += len(c)
	}
	return res
}

// StartLoop executes the background audio loop reading from arecord on Linux/RPi.
func (v *VAD) StartLoop(ctx context.Context) {
	log.Println("[VAD] Microphone monitor loop active.")

	var cmd *exec.Cmd
	var stdout io.ReadCloser

	// State variables
	isRecording := false
	var recordedBytes []byte
	var silenceDuration time.Duration
	preRoll := newRingBuffer(6) // 300ms pre-roll (6 chunks of 50ms)

	// Buffer for reading 50ms chunk (1600 bytes at 16kHz, 16-bit mono)
	buf := make([]byte, 1600)

	cleanupCmd := func() {
		if cmd != nil {
			log.Println("[VAD] Stopping arecord process...")
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			_ = cmd.Wait()
			cmd = nil
			stdout = nil
		}
		isRecording = false
		recordedBytes = nil
		silenceDuration = 0
	}
	defer cleanupCmd()

	for {
		select {
		case <-ctx.Done():
			log.Println("[VAD] Monitor loop terminated.")
			return
		default:
		}

		v.mu.Lock()
		muted := v.isMuted
		v.mu.Unlock()

		speaking := v.isSpeaking()

		if muted || speaking {
			cleanupCmd()
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// On Windows, run in mock mode
		if runtime.GOOS == "windows" {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		// Ensure arecord is running
		if cmd == nil {
			log.Println("[VAD] Starting arecord subprocess...")
			// capture 16kHz, 16-bit mono raw PCM stream
			cmd = exec.CommandContext(ctx, "arecord", "-f", "S16_LE", "-r", "16000", "-c", "1", "-t", "raw")
			var err error
			stdout, err = cmd.StdoutPipe()
			if err != nil {
				log.Printf("[VAD] Failed to get stdout pipe: %v", err)
				cmd = nil
				time.Sleep(1 * time.Second)
				continue
			}
			if err := cmd.Start(); err != nil {
				log.Printf("[VAD] Failed to start arecord: %v", err)
				stdout = nil
				cmd = nil
				time.Sleep(1 * time.Second)
				continue
			}
		}

		// Read 50ms chunk (blocks until data is ready or pipe is closed)
		_, err := io.ReadFull(stdout, buf)
		if err != nil {
			log.Printf("[VAD] Error reading from arecord: %v. Restarting...", err)
			cleanupCmd()
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// Calculate Root Absolute Average amplitude
		var sum int64
		for i := 0; i < len(buf); i += 2 {
			sample := int16(buf[i]) | (int16(buf[i+1]) << 8)
			val := int64(sample)
			if val < 0 {
				val = -val
			}
			sum += val
		}
		avgAmplitude := sum / int64(len(buf)/2)

		threshold := int64(1000) // Default speech detection threshold

		if avgAmplitude > threshold {
			if !isRecording {
				log.Printf("[VAD] Speech activity detected (amplitude: %d > %d). Recording hands-free...", avgAmplitude, threshold)
				isRecording = true
				recordedBytes = append([]byte(nil), preRoll.Flatten()...)
				silenceDuration = 0
			}

			chunkCopy := make([]byte, len(buf))
			copy(chunkCopy, buf)
			recordedBytes = append(recordedBytes, chunkCopy...)
			silenceDuration = 0
		} else {
			if isRecording {
				chunkCopy := make([]byte, len(buf))
				copy(chunkCopy, buf)
				recordedBytes = append(recordedBytes, chunkCopy...)

				silenceDuration += 50 * time.Millisecond
				if silenceDuration >= 1200*time.Millisecond {
					log.Printf("[VAD] Silence detected (sustained for %v). Processing recording of %d bytes...", silenceDuration, len(recordedBytes))

					// Prepend WAV header
					wavBytes := pcmToWav(recordedBytes, 16000)

					// Trigger callback asynchronously to prevent blocking the read loop
					go v.onSpeechRecorded(wavBytes)

					isRecording = false
					recordedBytes = nil
					silenceDuration = 0
				}
			} else {
				preRoll.Add(buf)
			}
		}
	}
}

// pcmToWav surrounds raw mono PCM bytes with a standard 16-bit WAV header.
func pcmToWav(pcmData []byte, sampleRate uint32) []byte {
	dataSize := uint32(len(pcmData))
	header := make([]byte, 44)
	copy(header[0:4], "RIFF")
	totalSize := 36 + dataSize
	header[4] = byte(totalSize)
	header[5] = byte(totalSize >> 8)
	header[6] = byte(totalSize >> 16)
	header[7] = byte(totalSize >> 24)
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	header[16] = 16
	header[17] = 0
	header[18] = 0
	header[19] = 0
	header[20] = 1 // PCM
	header[21] = 0
	header[22] = 1 // Mono (1 channel)
	header[23] = 0
	header[24] = byte(sampleRate)
	header[25] = byte(sampleRate >> 8)
	header[26] = byte(sampleRate >> 16)
	header[27] = byte(sampleRate >> 24)
	bytesPerSec := sampleRate * 2
	header[28] = byte(bytesPerSec)
	header[29] = byte(bytesPerSec >> 8)
	header[30] = byte(bytesPerSec >> 16)
	header[31] = byte(bytesPerSec >> 24)
	header[32] = 2  // Block align
	header[34] = 16 // Bits per sample
	copy(header[36:40], "data")
	header[40] = byte(dataSize)
	header[41] = byte(dataSize >> 8)
	header[42] = byte(dataSize >> 16)
	header[43] = byte(dataSize >> 24)
	return append(header, pcmData...)
}
