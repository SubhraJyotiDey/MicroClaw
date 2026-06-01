# MicroClaw (Gopal Bhar Edition) 🐾

MicroClaw is a lightweight, AI-powered hardware agent designed to run natively on a Raspberry Pi. It features the persona of **Gopal Bhar**, the legendary Bengali court jester—bringing intelligence, humor, and everyday wisdom to your smart home setup.

## Features ✨
- **Gopal Bhar Persona**: Interacts with wit and humor in Bengali, Hindi, or English.
- **Native Audio Support**: Uses ALSA (`arecord` and `aplay`) for direct microphone and speaker interaction on Linux/Raspberry Pi.
- **Hardware Control**: Actuates local network relays for smart devices like Kettles and Irrigation systems.
- **Vision Capabilities**: Captures images via USB webcam using `fswebcam` to "see" its surroundings.
- **Self-Healing Code Execution**: Can write, execute, and automatically fix Python scripts.
- **Multi-Step Reasoning**: Capable of evaluating tool outputs and planning subsequent actions before speaking.

## Hardware Requirements 🖥️
- **Raspberry Pi** (or any Linux machine).
- **USB Microphone & Speaker** (configured with ALSA).
- **USB Webcam** (for vision).
- **Network Relays** (for hardware control).

## Software Dependencies 📦
- **Go** (1.20+)
- **Python** (3.8+)
- **ALSA Utilities** (`alsa-utils` for `arecord` and `aplay`)
- **mpg123** (for streaming/decoding MP3 audio from Edge-TTS)
- **espeak-ng** (to optionally regenerate localized filler WAV assets)
- **fswebcam** (for taking pictures)

### Installing Dependencies on Raspberry Pi
```bash
sudo apt-get update
sudo apt-get install alsa-utils fswebcam mpg123 espeak-ng python3
```

## Setup & Installation 🚀

1. **Clone the Repository**
   ```bash
   git clone https://github.com/SubhraJyotiDey/MicroClaw.git
   cd MicroClaw
   ```

2. **Configure Environment Variables**
   Create a `.env` file in the `jarvis-core` directory (or wherever the executable runs) and add your API keys and configuration:
   ```env
   # API Keys
   OPENROUTER_API_KEY=your_openrouter_api_key
   GROQ_API_KEY=your_groq_api_key

   # API Endpoints
   API_URL_LLM=https://openrouter.ai/api/v1/chat/completions
   API_URL_STT=https://api.groq.com/openai/v1/audio/transcriptions

   # Hardware Configuration
   KETTLE_IP=192.168.1.50
   IRRIG_IP=192.168.1.51
   ```

3. **Build the Application**
   ```bash
   cd jarvis-core
   go build -o microclaw
   ```

4. **Set Up Filler Audio Assets (Optional)**
   The repository includes pre-built filler audio files. To regenerate or customize them, make sure `espeak-ng` is installed and run:
   ```bash
   mkdir -p assets
   espeak-ng -v bn -w assets/hmm_bn.wav "এক মুহূর্ত"
   espeak-ng -v hi -w assets/hmm_hi.wav "एक पल"
   espeak-ng -v en -w assets/hmm_en.wav "one moment"
   ```

5. **Run MicroClaw**
   ```bash
   ./microclaw
   ```

## Usage 🛠️
MicroClaw operates in an interactive CLI REPL. Type your commands or queries directly into the terminal.

Example:
```
USER: Turn on the kettle.
Gopal Bhar: *Thinking...* 
[Executing hardware tool...]
Gopal Bhar: Mashai, the kettle is on! A hot cup of tea will be ready soon, just like my sharp wit!
```
