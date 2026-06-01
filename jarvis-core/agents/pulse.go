package agents

import (
	"context"
	"log"
	"math/rand"
	"time"
)

// PulseAgent executes background system cron loops, scheduling health and sensor sweeps.
type PulseAgent struct {
	memory *MemoryAgent
}

// NewPulseAgent initializes a new PulseAgent instance.
func NewPulseAgent(memory *MemoryAgent) *PulseAgent {
	return &PulseAgent{
		memory: memory,
	}
}

// StartLoops launches background timers that trigger sensor reads.
// Listeners respond cleanly to <-ctx.Done() to block goroutine leaks.
func (p *PulseAgent) StartLoops(ctx context.Context, checkInterval time.Duration) {
	log.Printf("[PulseAgent] Background clock tickers initialized (interval=%v).", checkInterval)
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	// Initialize local thread-safe random number generator
	src := rand.NewSource(time.Now().UnixNano())
	rng := rand.New(src)

	for {
		select {
		case <-ctx.Done():
			log.Println("[PulseAgent] Background loops halted.")
			return
		case <-ticker.C:
			log.Println("[PulseAgent] Executing sweep schedule...")
			p.performSweep(rng)
		}
	}
}

func (p *PulseAgent) performSweep(rng *rand.Rand) {
	if p.memory == nil {
		return
	}

	// 1. Simulate soil moisture sensor (30% to 75%)
	moisture := 32.0 + rng.Float64()*40.0
	if err := p.memory.LogSensor("soil_moisture", moisture); err != nil {
		log.Printf("[PulseAgent] Failed database write for moisture sensor: %v", err)
	} else {
		log.Printf("[PulseAgent] Swept: Balcony Soil Moisture = %.1f%%", moisture)
	}

	// 2. Simulate Kettle temperature sensor (20C to 35C idle)
	temp := 21.0 + rng.Float64()*8.0
	if err := p.memory.LogSensor("kettle_temp", temp); err != nil {
		log.Printf("[PulseAgent] Failed database write for kettle temperature: %v", err)
	} else {
		log.Printf("[PulseAgent] Swept: Kettle Temperature = %.1f C", temp)
	}

	// Flag warning thresholds to prompt proactive irrigation
	if moisture < 35.0 {
		log.Printf("[PulseAgent] WARNING: Balcony soil moisture critical (%.1f%%). Trigger irrigation signal.", moisture)
	}
}
