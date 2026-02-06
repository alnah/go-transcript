# Architecture

## Pattern

**CLI Pipeline** with **Dependency Injection** for testability and **Multi-Provider** support for LLM restructuring.

```
                              main.go
                                 │
              ┌──────────────────┼──────────────────┐
              ▼                  ▼                  ▼
           record            transcribe           live
          (cli/record.go)   (cli/transcribe.go)  (cli/live.go)
              │                  │                  │
              ▼                  ▼                  ▼
           ┌──────────────────────────────────────────┐
           │                   Env                    │
           │  (Dependency Injection Container)        │
           │  - FFmpegResolver                        │
           │  - TranscriberFactory                    │
           │  - RestructurerFactory                   │
           │  - ChunkerFactory                        │
           │  - RecorderFactory                       │
           └──────────────────────────────────────────┘
                    │              │              │
                    ▼              ▼              ▼
                 audio         transcribe     restructure
               (internal/)    (internal/)    (internal/)
```

- **CLI Commands** - Cobra commands at `internal/cli/`
- **Env Container** - Dependency injection for all factories
- **Factories** - Abstract creation of domain objects for testing
- **Domain Packages** - Pure logic in `internal/{audio,transcribe,restructure,...}`

---

## Package Structure

See [LAYOUT.md](LAYOUT.md) for the complete project layout.

**Design decisions**:
- Single binary CLI at `cmd/transcript/`
- All business logic in `internal/` (not importable)
- Factories enable complete test isolation
- Multi-provider support via `RestructurerFactory`
- All API calls use `net/http` directly (no external SDK)
- Shared error sentinels and retry in `internal/apierr`

**Dependency direction**:
```
       cli
      /   \
     v     v
transcribe  restructure
     \     /
      v   v
      apierr          <-- stdlib only, no external deps
```

---

## Data Flow

### Live Command (Record + Transcribe)

```
Microphone ──▶ FFmpeg ──▶ OGG file ──▶ Chunker ──▶ Transcriber ──▶ Restructurer ──▶ Markdown
     │            │           │            │             │              │              │
   device     record.go    temp file   silences      OpenAI API     DeepSeek/     output.md
   loopback   recorder.go  or final    detection     parallel       OpenAI
   mix                                              (1-10 workers)
```

### Pipeline Stages

| Stage           | Input → Output           | Location              | Purpose                         |
| --------------- | ------------------------ | --------------------- | ------------------------------- |
| **record**      | Audio device → OGG       | `internal/audio/`     | FFmpeg recording                |
| **chunk**       | OGG → []ChunkPath        | `internal/audio/`     | Split at silences (<25MB)       |
| **transcribe**  | []ChunkPath → []Text     | `internal/transcribe/`| Parallel OpenAI API calls       |
| **restructure** | Text → Markdown          | `internal/restructure/`| Template-based LLM formatting  |
| **write**       | Markdown → File          | `internal/cli/`       | Atomic file write               |

---

## Dependency Injection

The `Env` struct centralizes all injectable dependencies:

```go
type Env struct {
    // I/O
    Stderr io.Writer
    Getenv func(string) string
    Now    func() time.Time

    // Factories
    FFmpegResolver      FFmpegResolver
    ConfigLoader        ConfigLoader
    TranscriberFactory  TranscriberFactory
    RestructurerFactory RestructurerFactory
    ChunkerFactory      ChunkerFactory
    RecorderFactory     RecorderFactory
}
```

**Benefits**:
- Commands testable in isolation
- No global state
- Time/environment mockable
- External services replaceable

**Usage in tests**:
```go
env := cli.NewEnv(
    cli.WithGetenv(func(k string) string { return "test-key" }),
    cli.WithTranscriberFactory(&mockTranscriberFactory{}),
)
```

---

## Multi-Provider Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                      RestructurerFactory                         │
│                                                                  │
│   NewMapReducer(provider, apiKey, opts...)                       │
│         │                                                        │
│         ├── "deepseek" ──▶ DeepSeekRestructurer                  │
│         │                  (internal/restructure/deepseek.go)    │
│         │                                                        │
│         └── "openai" ───▶ OpenAIRestructurer                     │
│                           (internal/restructure/openai.go)       │
└──────────────────────────────────────────────────────────────────┘
```

- **Provider selection** - Via `--provider` flag (default: deepseek)
- **Unified interface** - `MapReducer` handles both providers
- **MapReduce pattern** - Long transcripts split, processed, merged

---

## Audio Recording

```
┌──────────────────────────────────────────────────────────┐
│                    RecorderFactory                       │
│                                                          │
│   NewRecorder(ffmpegPath, device)      ──▶ Microphone    │
│   NewLoopbackRecorder(ctx, ffmpegPath) ──▶ System audio  │
│   NewMixRecorder(ctx, ffmpegPath, mic) ──▶ Both mixed    │
└──────────────────────────────────────────────────────────┘
         │
         ▼
┌──────────────────────────────────────────────────────────┐
│                    FFmpeg Process                        │
│                                                          │
│   Output: OGG Vorbis (16kHz mono, ~50kbps)               │
│   Optimized for voice, respects OpenAI's 25MB limit      │
└──────────────────────────────────────────────────────────┘
```

Platform-specific device detection:
- **macOS**: Core Audio, BlackHole for loopback
- **Linux**: PulseAudio/PipeWire monitor devices
- **Windows**: DirectShow, Stereo Mix or VB-Cable

---

## Chunking Strategy

```
┌───────────────────────────────────────────────────────────┐
│                 Silence-Based Chunking                    │
│                                                           │
│   Audio ──▶ FFmpeg silencedetect ──▶ Split at pauses      │
│                                                           │
│   Constraints:                                            │
│   - Max chunk size: 25MB (OpenAI limit)                   │
│   - Min silence: 0.5s                                     │
│   - Silence threshold: -30dB                              │
│                                                           │
│   Fallback: Force split at size limit if no silence found │
└───────────────────────────────────────────────────────────┘
```

---

## Transcription Pipeline

```
┌────────────────────────────────────────────────────────────┐
│                  Parallel Transcription                    │
│                                                            │
│   chunks[]  ──▶  Worker Pool (1-10)  ──▶  results[]        │
│                       │                                    │
│                       ▼                                    │
│              ┌─────────────────┐                           │
│              │  OpenAI API     │                           │
│              │  (direct HTTP)  │                           │
│              │  - Transcribe   │                           │
│              │  - Diarization  │                           │
│              └─────────────────┘                           │
│                                                            │
│   Retry: Exponential backoff via apierr.RetryWithBackoff   │
│   Error: Partial results preserved on failure              │
└────────────────────────────────────────────────────────────┘
```

All API calls use `net/http` directly (no third-party SDK). Each package defines
its own unexported `httpDoer` interface for testability via `httptest.Server`.

---

## MapReduce Restructuring

For long transcripts that exceed token limits:

```
┌──────────────────────────────────────────────────────────┐
│                    MapReduce Pattern                     │
│                                                          │
│   Transcript ──▶ Split into parts ──▶ Map (parallel)     │
│                                            │             │
│                                            ▼             │
│                              ┌──────────────────────┐    │
│                              │  Part 1 → Summary 1  │    │
│                              │  Part 2 → Summary 2  │    │
│                              │  Part N → Summary N  │    │
│                              └──────────────────────┘    │
│                                            │             │
│                                            ▼             │
│                                    Reduce (merge)        │
│                                            │             │
│                                            ▼             │
│                                   Final Markdown         │
└──────────────────────────────────────────────────────────┘
```

---

## Interfaces

### Transcription

| Interface     | Method                                  | Purpose                |
| ------------- | --------------------------------------- | ---------------------- |
| `Transcriber` | `Transcribe(ctx, path, opts) (string, error)` | Single chunk transcription |

### Restructuring

| Interface    | Method                                              | Purpose                |
| ------------ | --------------------------------------------------- | ---------------------- |
| `MapReducer` | `Restructure(ctx, text, template, lang) (string, bool, error)` | Full restructuring     |

### Audio

| Interface  | Method                                  | Purpose                |
| ---------- | --------------------------------------- | ---------------------- |
| `Recorder` | `Record(ctx, duration, output) error`   | Audio capture          |
| `Chunker`  | `Chunk(ctx, input) ([]string, error)`   | Silence-based splitting|

### Configuration

| Interface      | Method                        | Purpose                |
| -------------- | ----------------------------- | ---------------------- |
| `ConfigLoader` | `Load() (Config, error)`      | Load user settings     |

---

## Error Handling

```
┌──────────────────────────────────────────────────────────┐
│                      Error Types                         │
├──────────────────────────────────────────────────────────┤
│  cli.ErrAPIKeyMissing       - OPENAI_API_KEY not set     │
│  cli.ErrDeepSeekKeyMissing  - DEEPSEEK_API_KEY not set   │
│  cli.ErrInvalidProvider     - Invalid provider name      │
│  cli.ErrInvalidDuration     - Bad duration format        │
│  cli.ErrUnsupportedFormat   - Unknown audio format       │
│  cli.ErrFileNotFound        - Input file missing         │
│  cli.ErrOutputExists        - Output would overwrite     │
│  audio.ErrNoAudioDevice     - No recording device        │
│  audio.ErrLoopbackNotFound  - Loopback not available     │
│  apierr.ErrRateLimit        - API rate limited           │
│  apierr.ErrQuotaExceeded    - Billing issue              │
│  apierr.ErrTimeout          - Request timeout            │
│  apierr.ErrAuthFailed       - Invalid API key            │
│  apierr.ErrBadRequest       - Client error (4xx)         │
│  restructure.ErrTranscriptTooLong - Token limit exceeded │
│  template.ErrUnknown        - Invalid template name      │
└──────────────────────────────────────────────────────────┘
```

API error sentinels live in `internal/apierr`, shared across all providers.
Each provider (OpenAI transcription, OpenAI restructure, DeepSeek) maps its
HTTP response codes to these sentinels at the boundary. Retry logic uses
`apierr.RetryWithBackoff` with provider-specific `shouldRetry` predicates.

**Exit codes** map errors to specific values (see README.md).

---

## Interrupt Handling

```
┌───────────────────────────────────────────────────────┐
│                  Graceful Interrupts                  │
│                                                       │
│   First Ctrl+C:                                       │
│   └── Stop recording, continue with transcription     │
│                                                       │
│   Second Ctrl+C (within 2s):                          │
│   └── Abort entirely                                  │
│                                                       │
│   Implementation: internal/interrupt/handler.go       │
└───────────────────────────────────────────────────────┘
```

---

## Adding Features

| Feature Type       | Location                     | Example                     |
| ------------------ | ---------------------------- | --------------------------- |
| New CLI command    | `internal/cli/{cmd}.go`      | Add `export` command        |
| New provider       | `internal/restructure/`      | Add Anthropic provider      |
| New template       | `internal/template/`         | Add `interview` template    |
| New audio source   | `internal/audio/`            | Add Bluetooth capture       |
| New config key     | `internal/config/`           | Add `default-template`      |

**Checklist for new commands**:
1. Create `internal/cli/{cmd}.go` with `{Cmd}Cmd(env *Env)`
2. Add to `rootCmd.AddCommand()` in `cmd/transcript/main.go`
3. Handle errors with appropriate exit codes
4. Add tests in `internal/cli/{cmd}_test.go`
5. Document in README.md

**Checklist for new providers**:
1. Create `internal/restructure/{provider}.go` with direct HTTP calls
2. Implement the `Restructurer` interface (defined in `restructure.go`)
3. Define per-provider error type, classify into `apierr` sentinels
4. Add to `defaultRestructurerFactory.NewMapReducer()`
5. Add provider constant to `internal/cli/env.go`
6. Update CLI flag descriptions
7. Add unit tests using `httptest.Server`
8. Add integration tests
