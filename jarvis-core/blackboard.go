package main

import (
	"sync"
)

// Blackboard holds shared memory and state variables accessed concurrently by multiple agents.
type Blackboard struct {
	mu               sync.RWMutex
	isJarvisSpeaking bool
	isListening      bool
	sessionID        string
	hardwareState    map[string]interface{}
	lastLang         string
}

// NewBlackboard initializes a new Blackboard instance.
func NewBlackboard() *Blackboard {
	return &Blackboard{
		hardwareState: make(map[string]interface{}),
		lastLang:      "bn", // Default to Bengali
	}
}

// IsJarvisSpeaking returns the active playback state of Jarvis.
func (b *Blackboard) IsJarvisSpeaking() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.isJarvisSpeaking
}

// SetJarvisSpeaking updates the active playback state of Jarvis.
func (b *Blackboard) SetJarvisSpeaking(speaking bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.isJarvisSpeaking = speaking
}

// IsListening returns the active VAD/mic recording state.
func (b *Blackboard) IsListening() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.isListening
}

// SetListening updates the active VAD/mic recording state.
func (b *Blackboard) SetListening(listening bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.isListening = listening
}

// GetSessionID retrieves the current conversation session ID.
func (b *Blackboard) GetSessionID() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.sessionID
}

// SetSessionID updates the current conversation session ID.
func (b *Blackboard) SetSessionID(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sessionID = id
}

// GetHardwareState returns the state value for a given key.
func (b *Blackboard) GetHardwareState(key string) (interface{}, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	val, ok := b.hardwareState[key]
	return val, ok
}

// SetHardwareState sets a state value for a hardware key.
func (b *Blackboard) SetHardwareState(key string, val interface{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.hardwareState[key] = val
}

// GetLastLang returns the last response language ISO tag.
func (b *Blackboard) GetLastLang() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.lastLang == "" {
		return "bn"
	}
	return b.lastLang
}

// SetLastLang updates the last response language ISO tag.
func (b *Blackboard) SetLastLang(lang string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastLang = lang
}
