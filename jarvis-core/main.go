package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"jarvis-core/agents"
	"jarvis-core/audio"

	"github.com/joho/godotenv"
)

func main() {
	log.Println("[Main] Initializing Gopal Bhar RPi Interface (Fire Architecture)...")

	// Load variables from .env
	_ = godotenv.Load()

	// Configure root cancellation context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle OS shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		fmt.Printf("\n[System] Trapped shutdown signal: %v. Cleaning up...\n", sig)
		cancel()
	}()

	// Initialize Shared Blackboard State
	bb := NewBlackboard()
	bb.SetSessionID("session_rpi_gopal_bhar_cli")

	// Initialize database memory
	dbPath := "jarvis.db"
	memory, err := agents.NewMemoryAgent(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize SQLite database: %v", err)
	}
	defer memory.Close()

	// Load configuration parameters
	apiKey := os.Getenv("GROQ_API_KEY")
	sttURL := os.Getenv("API_URL_STT")
	llmURL := os.Getenv("API_URL_LLM")
	kettleIP := os.Getenv("KETTLE_IP")
	irrigIP := os.Getenv("IRRIG_IP")

	llmKey := apiKey
	if strings.Contains(llmURL, "openrouter.ai") {
		if orKey := os.Getenv("OPENROUTER_API_KEY"); orKey != "" {
			llmKey = orKey
		}
	}

	if llmKey == "" {
		log.Println("[Warning] No API keys found. System will run in mock mode.")
	}

	// Initialize audio components
	player := audio.NewPlayer(bb.SetJarvisSpeaking)
	sttClient := audio.NewSTTClient(apiKey, sttURL)
	ttsClient := audio.NewTTSClient()

	// Initialize execution agents
	hwChan := make(chan agents.HardwareCommand, 100)
	relayToken := os.Getenv("RELAY_TOKEN")
	hwAgent := agents.NewHardwareAgent(kettleIP, irrigIP, relayToken, memory)
	go hwAgent.StartLoop(ctx, hwChan)

	coderAgent, err := agents.NewCoderAgent("sandbox")
	if err != nil {
		log.Fatalf("Failed to initialize Python sandbox: %v", err)
	}
	healerAgent := agents.NewHealerAgent(coderAgent, llmKey, llmURL)
	visionAgent := agents.NewVisionAgent(apiKey, llmURL)
	skillsManager := agents.NewSkillsManager(healerAgent, visionAgent)

	// Context and Router
	cm := NewContextManager()
	router := NewRouter(hwChan, llmKey, llmURL, GetSystemPrompt())

	// Default voice setting
	gender := "female"

	fmt.Println("\n=============================================================")
	fmt.Println("    PROJECT JARVIS - GOPAL BHAR COURT JESTER SYSTEM")
	fmt.Println("=============================================================")
	fmt.Println("Commands:")
	fmt.Println("  Speak naturally (Hands-free voice detection is active!).")
	fmt.Println("  Type any message and press [Enter] to chat.")
	fmt.Println("  Type /speak to record voice from the microphone (5s).")
	fmt.Println("  Type /gender <male/female> to toggle TTS voice gender.")
	fmt.Println("  Type /quit or /exit to shut down.")
	fmt.Println("=============================================================\n")

	// Welcome chime & greeting
	welcomeText := "নমস্কার মহারাজ! আমি গোপাল ভাঁড়। মহারাজ কৃষ্ণচন্দ্রের রাজসভা ছেড়ে এখন আপনার কম্পিউটারে হাজির! বলুন, কী আদেশ আপনার?"
	fmt.Printf("Gopal Bhar > %s\n\n", welcomeText)
	speakAndPlay(ctx, ttsClient, player, welcomeText, "bn", gender)

	// Channel for inputs (both CLI and VAD voice transcription)
	inputChan := make(chan string, 10)

	// Background CLI scanner
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			text := strings.TrimSpace(scanner.Text())
			if text != "" {
				inputChan <- text
			}
		}
	}()

	// Background Hands-free VAD callback
	onSpeech := func(wavBytes []byte) {
		reader := bytes.NewReader(wavBytes)
		transcript, err := sttClient.Transcribe(ctx, reader, "auto")
		if err != nil {
			log.Printf("[VAD] STT transcription failed: %v", err)
			return
		}
		transcript = strings.TrimSpace(transcript)
		if transcript != "" {
			fmt.Printf("\rUser (Voice) > %s\n", transcript)
			inputChan <- transcript
		}
	}

	vad := audio.NewVAD(onSpeech, bb.IsJarvisSpeaking)
	go vad.StartLoop(ctx)

	for {
		fmt.Print("User > ")

		var input string
		select {
		case <-ctx.Done():
			break
		case input = <-inputChan:
		}

		if ctx.Err() != nil {
			break
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		// Handle Slash Commands
		if strings.HasPrefix(input, "/") {
			parts := strings.Split(input, " ")
			cmd := parts[0]

			if cmd == "/quit" || cmd == "/exit" {
				fmt.Println("[System] Leaving the Maharaja's court. Farewell!")
				break
			}

			if cmd == "/gender" {
				if len(parts) < 2 {
					fmt.Printf("[System] Current voice gender: %s. Use '/gender male' or '/gender female' to switch.\n", gender)
					continue
				}
				g := strings.ToLower(parts[1])
				if g == "male" || g == "female" {
					gender = g
					fmt.Printf("[System] Voice gender switched to: %s\n", gender)
				} else {
					fmt.Println("[System] Invalid voice gender. Use 'male' or 'female'.")
				}
				continue
			}

			if cmd == "/speak" {
				audioPath := "speech.wav"
				fmt.Println("[System] Speak now! Recording for 5 seconds...")

				// Record 5s WAV natively
				err := audio.RecordAudio(ctx, 5*time.Second, audioPath)
				if err != nil {
					fmt.Printf("[System] Error recording audio: %v\n", err)
					continue
				}

				fmt.Println("[System] Transcribing audio with auto-detection...")
				file, err := os.Open(audioPath)
				if err != nil {
					fmt.Printf("[System] Error reading recording: %v\n", err)
					os.Remove(audioPath)
					continue
				}

				transcript, err := sttClient.Transcribe(ctx, file, "auto")
				file.Close()
				os.Remove(audioPath)

				if err != nil {
					fmt.Printf("[System] STT Failed: %v\n", err)
					continue
				}

				transcript = strings.TrimSpace(transcript)
				if transcript == "" {
					fmt.Println("[System] No speech detected.")
					continue
				}

				fmt.Printf("User (Voice) > %s\n", transcript)
				input = transcript
			} else {
				fmt.Println("[System] Unknown command. Available: /speak, /gender <male/female>, /exit")
				continue
			}
		}

		// Process interaction through the router with dynamic multi-step skill loops
		cm.AddMessage("user", input)
		currentPrompt := input

		// Allow up to 3 sequential skill calls per turn (Agentic Loop)
		const fillerMaxDuration = 900 * time.Millisecond

		for attempt := 0; attempt < 3; attempt++ {
			// Play filler audio concurrently to mask LLM latency (localized using last response language)
			fillerCtx, fillerCancel := context.WithTimeout(ctx, fillerMaxDuration)
			fillerFile := fmt.Sprintf("assets/hmm_%s.wav", bb.GetLastLang())
			go func(file string) {
				_ = player.PlayFiller(fillerCtx, file)
			}(fillerFile)

			resp, isFast, err := router.Route(ctx, currentPrompt, cm)
			fillerCancel()
			// Small yield to let filler goroutine release the Player mutex before PlayStream (only on slow path)
			if !isFast {
				time.Sleep(10 * time.Millisecond)
			}

			if err != nil {
				fmt.Printf("Gopal Bhar (Error) > ওরে বাবা! মাথা কাজ করছে না! (Error: %v)\n", err)
				break
			}

			bb.SetLastLang(resp.LanguageCode)

			// Append LLM reply to context
			respJSON, _ := json.Marshal(resp)
			cm.AddMessage("assistant", string(respJSON))

			// Check if a skill was requested
			if resp.SkillName == "" {
				source := "BRAIN"
				if isFast {
					source = "FAST PATH"
				}
				fmt.Printf("Gopal Bhar (%s) > %s\n\n", source, resp.Text)
				speakAndPlay(ctx, ttsClient, player, resp.Text, resp.LanguageCode, gender)
				break
			}

			// Skill triggered!
			source := "BRAIN"
			if isFast {
				source = "FAST PATH"
			}
			fmt.Printf("Gopal Bhar (%s) [Skill: %s] > %s\n", source, resp.SkillName, resp.Text)
			speakAndPlay(ctx, ttsClient, player, resp.Text, resp.LanguageCode, gender)

			// Execute skill
			output, err := skillsManager.Execute(ctx, resp.SkillName, resp.SkillArgs)
			if err != nil {
				output = fmt.Sprintf("Error executing skill: %v", err)
			}

			fmt.Printf("\n[Skill Output]\n%s\n\n", output)

			// Append tool output to context
			cm.AddMessage("system", fmt.Sprintf("[Tool Output for %s]: %s", resp.SkillName, output))

			// Nudge LLM to synthesize final response
			currentPrompt = "Formulate your final conversational response now that you have the tool execution output."
		}

		// Compress raw context history in background if needed
		go func() {
			if err := cm.CompressIfNeeded(context.Background(), apiKey, llmURL); err != nil {
				log.Printf("[Context Manager] Background compression failed: %v", err)
			}
		}()
	}

	log.Println("[Main] Gopal Bhar CLI shutdown finalized.")
}

func speakAndPlay(ctx context.Context, tts *audio.TTSClient, player *audio.Player, text string, lang string, gender string) {
	// Chunk text into sentences for sub-second first-audio latency
	chunks := audio.ChunkText(text)
	if len(chunks) == 0 {
		return
	}

	// For short responses (1-2 sentences), process as a single chunk to avoid extra network round-trips
	if len(chunks) <= 2 {
		chunks = []string{text}
	}

	for _, chunk := range chunks {
		pr, pw := io.Pipe()

		go func(c string) {
			err := tts.StreamSpeech(ctx, c, lang, gender, pw)
			if err != nil {
				log.Printf("[TTS ERROR] %v", err)
				_ = pw.CloseWithError(err)
			} else {
				_ = pw.Close()
			}
		}(chunk)

		err := player.PlayStream(ctx, pr)
		if err != nil {
			log.Printf("[PLAYBACK ERROR] %v", err)
		}
		_ = pr.Close()
	}
}
