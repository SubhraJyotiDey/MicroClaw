package audio

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"time"
)

// RecordAudio runs arecord to capture microphone input into a WAV file.
// If running on Windows, it outputs a mock logging statement and writes a dummy quiet WAV.
func RecordAudio(ctx context.Context, duration time.Duration, outputPath string) error {
	log.Printf("[Recorder] Recording audio for %v to %s...", duration, outputPath)

	if runtime.GOOS == "windows" {
		log.Println("[Recorder] [Mock] Simulating audio recording (Windows environment).")
		time.Sleep(duration)
		
		// Write a valid small dummy WAV (16kHz 16-bit mono PCM) so API doesn't throw 400
		dataSize := uint32(16000 * 2 * duration.Seconds())
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
		header[20] = 1  // PCM
		header[22] = 1  // Mono
		header[24] = 0x80 // 16000 Hz
		header[25] = 0x3E
		header[28] = 0x00 // 32000 Bytes/sec
		header[29] = 0x7D
		header[32] = 2  // Block align
		header[34] = 16 // Bits per sample
		copy(header[36:40], "data")
		header[40] = byte(dataSize)
		header[41] = byte(dataSize >> 8)
		header[42] = byte(dataSize >> 16)
		header[43] = byte(dataSize >> 24)

		dummyBytes := append(header, make([]byte, dataSize)...)
		if err := os.WriteFile(outputPath, dummyBytes, 0644); err != nil {
			return fmt.Errorf("failed to save mock WAV file: %w", err)
		}
		return nil
	}

	// On Linux/RPi, use arecord: arecord -d <sec> -f S16_LE -r 16000 -t wav <path>
	durSeconds := fmt.Sprintf("%d", int(duration.Seconds()))
	if durSeconds == "0" {
		durSeconds = "1"
	}
	cmd := exec.CommandContext(ctx, "arecord", "-d", durSeconds, "-f", "S16_LE", "-r", "16000", "-t", "wav", outputPath)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("arecord execution failed: %w", err)
	}

	log.Println("[Recorder] Recording complete.")
	return nil
}
