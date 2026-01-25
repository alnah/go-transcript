# go-transcript

CLI tool for audio transcription using OpenAI Whisper API.

## Installation

```bash
go install github.com/alnah/go-transcript@latest
```

## Requirements

- Go 1.25+
- FFmpeg (auto-downloaded if not present)
- OpenAI API key

## Usage

```bash
# Set your API key
export OPENAI_API_KEY=your-key

# Record and transcribe
transcript live -d 30s -o notes.md

# Transcribe existing audio
transcript transcribe audio.mp3 -o output.md

# Record only
transcript record -d 10s -o recording.ogg
```

## Development

```bash
make build    # Build binary
make test     # Run tests
make check    # Run all checks (fmt, vet, lint, test)
```

## CI/CD Status

- **Tests**: Enforced
- **Formatting**: Enforced (gofmt)
- **Static analysis**: Enforced (go vet, staticcheck)
- **Security scan**: Advisory (gosec findings to be addressed)

## License

BSD 3-Clause - see [LICENSE](LICENSE)
