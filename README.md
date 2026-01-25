# go-transcript

[![Go Reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white)](https://pkg.go.dev/github.com/alnah/go-transcript)
[![Go Report Card](https://goreportcard.com/badge/github.com/alnah/go-transcript)](https://goreportcard.com/report/github.com/alnah/go-transcript)
[![Build Status](https://img.shields.io/github/actions/workflow/status/alnah/go-transcript/ci.yml?branch=main)](https://github.com/alnah/go-transcript/actions)
[![Coverage](https://img.shields.io/codecov/c/github/alnah/go-transcript)](https://codecov.io/gh/alnah/go-transcript)
[![License](https://img.shields.io/badge/License-BSD_3--Clause-blue.svg)](LICENSE)

> Record, transcribe, and restructure audio using OpenAI's transcription API with automatic chunking and template-based formatting.

## Features

- **Record audio** from microphone, system audio (loopback), or both mixed
- **Transcribe** audio files with automatic silence-based chunking (respects OpenAI's 25MB limit)
- **Restructure** transcripts using templates: `brainstorm`, `meeting`, `lecture`
- **Live mode** combines recording and transcription in one command with graceful interrupt handling
- **Auto-download FFmpeg** if not installed (macOS, Linux, Windows)

## Installation

```bash
go install github.com/alnah/go-transcript@latest
```

Or build from source:

```bash
git clone https://github.com/alnah/go-transcript.git
cd go-transcript
make build
```

## Requirements

- Go 1.25+
- FFmpeg (auto-downloaded if not present)
- OpenAI API key (`OPENAI_API_KEY` environment variable)

## Quick Start

```bash
# Set your API key
export OPENAI_API_KEY=sk-...

# Record a meeting and get structured notes
transcript live -d 1h -o meeting.md -t meeting

# Transcribe an existing recording
transcript transcribe recording.ogg -o notes.md -t brainstorm

# Record system audio (e.g., video call)
transcript record -d 30m --loopback -o call.ogg
```

## Usage

### Record

Record audio from microphone, system audio, or both.

```bash
# Record from microphone for 2 hours
transcript record -d 2h -o session.ogg

# Record system audio (requires BlackHole on macOS, PulseAudio on Linux)
transcript record -d 30m --loopback -o system.ogg

# Record both microphone and system audio mixed
transcript record -d 1h --mix -o meeting.ogg
```

### Transcribe

Transcribe an existing audio file.

```bash
# Basic transcription
transcript transcribe audio.ogg -o transcript.md

# With template restructuring
transcript transcribe lecture.mp3 -o notes.md -t lecture

# Specify audio language for better accuracy
transcript transcribe french_audio.ogg -o notes.md -l fr

# Transcribe French audio, output in English
transcript transcribe french_audio.ogg -o notes.md -l fr --output-lang en -t meeting
```

### Live

Record and transcribe in one step. Press Ctrl+C to stop recording early and continue with transcription.

```bash
# Basic live transcription
transcript live -d 30m -o notes.md

# With template and keep the audio file
transcript live -d 1h -o meeting.md -t meeting --keep-audio

# Record video call (both sides) with speaker identification
transcript live -d 2h --mix -t meeting --diarize -o call.md
```

### Config

Manage persistent configuration.

```bash
# Set default output directory
transcript config set output-dir ~/Documents/transcripts

# View current configuration
transcript config list
```

## Templates

| Template | Purpose | Output Structure |
|----------|---------|------------------|
| `brainstorm` | Idea generation sessions | H1 topic, H2 themes, bullet points, key insights, actions |
| `meeting` | Meeting notes | H1 subject, participants, topics discussed, decisions, action items |
| `lecture` | Course/conference notes | H1 subject, H2 concepts, definitions in bold, key quotes |

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `OPENAI_API_KEY` | Yes | - | OpenAI API key for transcription and restructuring |
| `TRANSCRIPT_OUTPUT_DIR` | No | `.` | Default output directory for generated files |
| `FFMPEG_PATH` | No | auto | Path to FFmpeg binary (auto-downloaded if not set) |

### Config File

Configuration is stored in `~/.config/go-transcript/config` (or `$XDG_CONFIG_HOME/go-transcript/config`).

| Key | Description |
|-----|-------------|
| `output-dir` | Default directory for output files |

## Command Reference

### Common Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--output` | `-o` | Output file path |
| `--template` | `-t` | Restructure template: `brainstorm`, `meeting`, `lecture` |
| `--language` | `-l` | Audio language (ISO 639-1: `en`, `fr`, `pt-BR`) |
| `--output-lang` | - | Output language for restructured text (requires `--template`) |
| `--parallel` | `-p` | Max concurrent API requests (1-10, default: 3) |
| `--diarize` | - | Enable speaker identification |

### Record Flags

| Flag | Description |
|------|-------------|
| `--duration`, `-d` | Recording duration (required): `30s`, `5m`, `2h` |
| `--device` | Specific audio input device |
| `--loopback` | Capture system audio instead of microphone |
| `--mix` | Capture both microphone and system audio |

### Live Flags

| Flag | Description |
|------|-------------|
| `--keep-audio` | Preserve the audio file after transcription |

## Supported Audio Formats

ogg, mp3, wav, m4a, flac, mp4, mpeg, mpga, webm

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Usage error (invalid flags) |
| 3 | Setup error (FFmpeg not found, API key missing, no audio device) |
| 4 | Validation error (unsupported format, file not found, invalid language) |
| 5 | Transcription error (rate limit, quota exceeded, auth failed) |
| 6 | Restructure error (transcript too long) |
| 130 | Interrupted (Ctrl+C) |

## Development

```bash
make build    # Build binary
make test     # Run tests
make check    # Run all checks (fmt, vet, lint, test)
make bench    # Run benchmarks
make tools    # Install staticcheck and gosec
```

## CI/CD

- **Tests**: Enforced with race detection
- **Coverage**: Uploaded to Codecov
- **Formatting**: Enforced (gofmt)
- **Static analysis**: Enforced (go vet, staticcheck)
- **Security scan**: Advisory (gosec)
- **Releases**: Automated via GoReleaser on tags

## License

BSD 3-Clause - see [LICENSE](LICENSE)
