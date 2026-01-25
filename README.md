# go-transcript

[![Go Reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white)](https://pkg.go.dev/github.com/alnah/go-transcript)
[![Go Report Card](https://img.shields.io/badge/go%20report-A+-brightgreen)](https://goreportcard.com/report/github.com/alnah/go-transcript)
[![Build Status](https://img.shields.io/github/actions/workflow/status/alnah/go-transcript/ci.yml?branch=main)](https://github.com/alnah/go-transcript/actions)
[![Coverage](https://img.shields.io/codecov/c/github/alnah/go-transcript)](https://codecov.io/gh/alnah/go-transcript)
[![License](https://img.shields.io/badge/License-BSD--3--Clause-blue.svg)](LICENSE)

> Record, transcribe, and restructure audio via CLI - microphone/loopback capture, automatic chunking, parallel transcription, and template-based formatting.

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Features](#features)
- [CLI Reference](#cli-reference)
- [Environment Variables](#environment-variables)
- [Configuration](#configuration)
- [Templates](#templates)
- [Troubleshooting](#troubleshooting)
- [Known Limitations](#known-limitations)
- [Contributing](#contributing)

## Installation

```bash
go install github.com/alnah/go-transcript@latest
```

<details>
<summary>Other installation methods</summary>

### Build from Source

```bash
git clone https://github.com/alnah/go-transcript.git
cd go-transcript
make build
```

### Binary Download

Download pre-built binaries from [GitHub Releases](https://github.com/alnah/go-transcript/releases).

</details>

## Requirements

- Go 1.25+
- FFmpeg (downloaded automatically on first run)
- OpenAI API key

> **Note:** FFmpeg is auto-downloaded for macOS (arm64/amd64), Linux (amd64), and Windows (amd64). Set `FFMPEG_PATH` to use a custom binary.

## Quick Start

```bash
# Set your API key
export OPENAI_API_KEY=sk-...

# Record and transcribe a meeting
transcript live -d 1h -o meeting.md -t meeting

# Transcribe an existing recording
transcript transcribe recording.ogg -o notes.md -t brainstorm

# Record system audio (video call)
transcript record -d 30m --loopback -o call.ogg
```

## Features

- **Audio recording** - Microphone, system audio (loopback), or both mixed
- **Automatic chunking** - Splits at silences to respect OpenAI's 25MB limit
- **Parallel transcription** - Concurrent API requests (configurable 1-10)
- **Template restructuring** - `brainstorm`, `meeting`, `lecture` formats
- **Language support** - Specify audio language, translate output
- **Graceful interrupts** - Ctrl+C stops recording, continues transcription

## CLI Reference

```bash
transcript <command> [flags]

Commands:
  record       Record audio to file
  transcribe   Transcribe audio file to text
  live         Record and transcribe in one step
  config       Manage configuration
  help         Help about any command
  version      Show version information
```

### record

Record audio from microphone, system audio, or both.

```bash
transcript record -d 2h -o session.ogg           # Microphone
transcript record -d 30m --loopback -o system.ogg # System audio
transcript record -d 1h --mix -o meeting.ogg      # Both mixed
```

<details>
<summary>All flags</summary>

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--duration` | `-d` | required | Recording duration (e.g., `30s`, `5m`, `2h`) |
| `--output` | `-o` | `recording_<timestamp>.ogg` | Output file path |
| `--device` | | system default | Specific audio input device |
| `--loopback` | | `false` | Capture system audio instead of microphone |
| `--mix` | | `false` | Capture both microphone and system audio |

`--loopback` and `--mix` are mutually exclusive.

</details>

### transcribe

Transcribe an existing audio file.

```bash
transcript transcribe audio.ogg -o notes.md
transcript transcribe lecture.mp3 -o notes.md -t lecture
transcript transcribe french.ogg -o notes.md -l fr --output-lang en -t meeting
```

<details>
<summary>All flags</summary>

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--output` | `-o` | `<input>.md` | Output file path |
| `--template` | `-t` | | Restructure template: `brainstorm`, `meeting`, `lecture` |
| `--language` | `-l` | auto-detect | Audio language (ISO 639-1: `en`, `fr`, `pt-BR`) |
| `--output-lang` | | same as input | Output language for restructured text |
| `--parallel` | `-p` | `3` | Max concurrent API requests (1-10) |
| `--diarize` | | `false` | Enable speaker identification |

`--output-lang` requires `--template`.

</details>

### live

Record and transcribe in one step. Press Ctrl+C to stop recording early and continue with transcription. Press Ctrl+C twice within 2 seconds to abort entirely.

```bash
transcript live -d 30m -o notes.md
transcript live -d 1h -o meeting.md -t meeting --keep-audio
transcript live -d 2h --mix -t meeting --diarize -o call.md
```

<details>
<summary>All flags</summary>

Inherits all flags from `record` and `transcribe`, plus:

| Flag | Default | Description |
|------|---------|-------------|
| `--keep-audio` | `false` | Preserve the audio file after transcription |

</details>

### config

Manage persistent configuration.

```bash
transcript config set output-dir ~/Documents/transcripts
transcript config get output-dir
transcript config list
```

<details>
<summary>Exit codes</summary>

| Code | Name | Description |
|------|------|-------------|
| 0 | Success | Operation completed successfully |
| 1 | General | Unexpected or unclassified error |
| 2 | Usage | Invalid flags or arguments |
| 3 | Setup | FFmpeg not found, API key missing, no audio device |
| 4 | Validation | Unsupported format, file not found, invalid language |
| 5 | Transcription | Rate limit, quota exceeded, auth failed |
| 6 | Restructure | Transcript exceeds token limit |
| 130 | Interrupt | Aborted via Ctrl+C |

</details>

## Environment Variables

**Priority:** CLI flags > environment variables > config file > defaults

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `OPENAI_API_KEY` | Yes | | OpenAI API key for transcription and restructuring |
| `TRANSCRIPT_OUTPUT_DIR` | No | `.` | Default output directory |
| `FFMPEG_PATH` | No | auto | Path to FFmpeg binary (skips auto-download) |

## Configuration

Config files are stored in the user config directory:

| OS | Config Directory |
|----|------------------|
| Linux | `~/.config/go-transcript/` |
| macOS | `~/.config/go-transcript/` |
| Windows | `%APPDATA%\go-transcript\` |

Respects `XDG_CONFIG_HOME` if set.

| Key | Description |
|-----|-------------|
| `output-dir` | Default directory for output files |

<details>
<summary>Example config file</summary>

```ini
# ~/.config/go-transcript/config
output-dir=/Users/john/Documents/transcripts
```

</details>

## Templates

Templates transform raw transcripts into structured markdown.

| Template | Purpose | Output Structure |
|----------|---------|------------------|
| `brainstorm` | Idea generation sessions | H1 topic, H2 themes, bullet points, key insights, actions |
| `meeting` | Meeting notes | H1 subject, participants, topics discussed, decisions, action items |
| `lecture` | Course/conference notes | H1 subject, H2 concepts, definitions in bold, key quotes |

Templates are in French by default. Use `--output-lang` to translate:

```bash
transcript transcribe audio.ogg -t meeting --output-lang en
```

## Supported Formats

OpenAI accepts: `ogg`, `mp3`, `wav`, `m4a`, `flac`, `mp4`, `mpeg`, `mpga`, `webm`

Recording output is always OGG Vorbis (16kHz mono, ~50kbps) optimized for voice.

## Troubleshooting

### FFmpeg not found

FFmpeg is auto-downloaded on first run. If download fails:

```bash
# macOS
brew install ffmpeg

# Ubuntu/Debian
sudo apt install ffmpeg

# Windows
winget install ffmpeg
```

Or set `FFMPEG_PATH` to your binary location.

### Loopback device not found

System audio capture requires a virtual audio driver:

<details>
<summary>macOS - BlackHole</summary>

```bash
brew install --cask blackhole-2ch
```

**Important:** BlackHole is a "black hole" - audio sent to it is NOT audible. To hear audio while recording:

1. Open "Audio MIDI Setup" (Spotlight search)
2. Click "+" > "Create Multi-Output Device"
3. Check both your speakers AND BlackHole 2ch
4. Set this Multi-Output as your system output

</details>

<details>
<summary>Linux - PulseAudio/PipeWire</summary>

Usually pre-installed. Loopback uses the monitor device of your default sink.

```bash
# Verify PulseAudio is working
pactl get-default-sink

# Install if missing
sudo apt install pulseaudio pulseaudio-utils
```

</details>

<details>
<summary>Windows - Stereo Mix or VB-Cable</summary>

**Option 1 - Enable Stereo Mix (recommended):**

1. Right-click speaker icon > Sound settings > More sound settings
2. Recording tab > Right-click > Show Disabled Devices
3. Enable "Stereo Mix" if present

**Option 2 - Install VB-Audio Virtual Cable:**

Download from: https://vb-audio.com/Cable/

</details>

### API errors

| Error | Cause | Solution |
|-------|-------|----------|
| "OPENAI_API_KEY not set" | Missing API key | `export OPENAI_API_KEY=sk-...` |
| "rate limit exceeded" | Too many requests | Reduce `--parallel` or wait |
| "quota exceeded" | Billing issue | Check OpenAI account billing |
| "authentication failed" | Invalid API key | Verify your API key |

### Transcript too long

The restructuring step has a ~100K token limit. For very long recordings:

- Skip restructuring (no `--template`)
- Split the audio file manually
- Use shorter recording sessions

## Known Limitations

### By Design

| Not Supported | Why |
|---------------|-----|
| Real-time streaming | OpenAI Whisper API is batch-only |
| Local transcription | Requires OpenAI API |
| Video input | Audio extraction not implemented |

### OpenAI API

| Limitation | Workaround |
|------------|------------|
| 25MB file size | Auto-chunking at silences |
| Rate limits | Exponential backoff with retry |
| No true diarization | Uses segment-based pseudo-diarization |

### Platform Notes

| Issue | Solution |
|-------|----------|
| No loopback on Linux without PulseAudio | Install pulseaudio |
| BlackHole mutes audio on macOS | Create Multi-Output Device |
| Stereo Mix disabled on Windows | Enable in Sound settings |

## Contributing

```bash
make build    # Build binary
make test     # Run tests
make check    # Run all checks (fmt, vet, lint, test)
make bench    # Run benchmarks
make tools    # Install staticcheck and gosec
```

## License

See: [BSD-3-Clause](LICENSE).
