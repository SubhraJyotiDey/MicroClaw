package agents

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// HardwareCommand encapsulates state control signals for kettles or irrigation valves.
type HardwareCommand struct {
	Device string // "kettle" or "irrigation"
	Action string // "on" or "off"
}

// HardwareAgent manages network REST commands directing Tasmota/Shelly/ESP32 relay nodes.
type HardwareAgent struct {
	kettleIP     string
	irrigationIP string
	authToken    string
	client       *http.Client
	memory       *MemoryAgent
}

// NewHardwareAgent constructs a new Hardware controller.
func NewHardwareAgent(kettleIP, irrigationIP, authToken string, memory *MemoryAgent) *HardwareAgent {
	return &HardwareAgent{
		kettleIP:     kettleIP,
		irrigationIP: irrigationIP,
		authToken:    authToken,
		memory:       memory,
		client: &http.Client{
			Timeout: 2 * time.Second,
		},
	}
}

// ControlDevice dispatches an HTTP REST call to the target ESP32 node.
func (h *HardwareAgent) ControlDevice(ctx context.Context, device, action string) error {
	ip := h.kettleIP
	if device == "irrigation" {
		ip = h.irrigationIP
	}

	if ip == "" {
		return fmt.Errorf("ip address for device %q not configured", device)
	}

	stateVal := "0"
	if action == "on" {
		stateVal = "1"
	}

	// Trigger standard REST API GET endpoint for ESP32 lab relays with auth token:
	url := fmt.Sprintf("http://%s/control?state=%s", ip, stateVal)
	if h.authToken != "" {
		url += "&token=" + h.authToken
	}
	log.Printf("[HardwareAgent] Swerving control request to URL: %s", url)

	// Mock bypass handling for test harnesses or localhost testing
	if strings.HasPrefix(ip, "mock") || ip == "127.0.0.1" {
		log.Printf("[HardwareAgent] [Mock] Switched %s to state %s successfully.", device, action)
		h.logStateToDB(device, action)
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := h.client.Do(req)
	if err != nil {
		h.logStateToDB(device, "error")
		return fmt.Errorf("esp32 connection timed out or unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		h.logStateToDB(device, "error")
		return fmt.Errorf("esp32 returned non-ok HTTP status code: %d", resp.StatusCode)
	}

	log.Printf("[HardwareAgent] ESP32 control confirmed for %s -> %s", device, action)
	h.logStateToDB(device, action)
	return nil
}

func (h *HardwareAgent) logStateToDB(device, action string) {
	if h.memory == nil {
		return
	}

	val := 0.0
	if action == "on" {
		val = 1.0
	} else if action == "error" {
		val = -1.0
	}

	if err := h.memory.LogSensor(device+"_state", val, false); err != nil {
		log.Printf("[HardwareAgent] Failed to log state change in SQLite: %v", err)
	}
}

// StartLoop monitors the channel for incoming commands and fires network tasks.
func (h *HardwareAgent) StartLoop(ctx context.Context, cmdChan <-chan HardwareCommand) {
	log.Println("[HardwareAgent] Command loop active. Waiting for signals...")
	for {
		select {
		case <-ctx.Done():
			log.Println("[HardwareAgent] Command loop terminated.")
			return
		case cmd, ok := <-cmdChan:
			if !ok {
				log.Println("[HardwareAgent] Command channel closed. Exiting.")
				return
			}
			log.Printf("[HardwareAgent] Processing channel trigger: %+v", cmd)
			// Execute sequentially in a separate context to prevent out-of-order hardware actions
			execCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			if err := h.ControlDevice(execCtx, cmd.Device, cmd.Action); err != nil {
				log.Printf("[HardwareAgent] Control action failed: %v", err)
			}
			cancel()
		}
	}
}
