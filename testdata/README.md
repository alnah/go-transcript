# Test Fixtures

This directory contains test fixtures for go-transcript integration tests.

## Audio Fixtures

### sample.ogg

Audio file (~8s) with detectable silence periods. Used to test `SilenceChunker` silence detection and cut point selection.

**Characteristics:**
- Format: OGG Vorbis, mono, 16kHz, ~21 KB
- Duration: 8 seconds
- Structure: 2s tone (440Hz) + 1s silence + 2s tone (880Hz) + 1s silence + 2s tone (660Hz)

**Expected silencedetect output** (with `noise=-30dB:d=0.5`):
- silence_start: ~2.0s, silence_end: ~3.0s
- silence_start: ~5.0s, silence_end: ~6.0s

**Generation command** (do not regenerate unless necessary):

```bash
ffmpeg -y \
  -f lavfi -i "sine=frequency=440:duration=2" \
  -f lavfi -i "anullsrc=r=16000:cl=mono" \
  -f lavfi -i "sine=frequency=880:duration=2" \
  -f lavfi -i "anullsrc=r=16000:cl=mono" \
  -f lavfi -i "sine=frequency=660:duration=2" \
  -filter_complex "[0]aresample=16000[s0];[1]atrim=duration=1[p1];[2]aresample=16000[s1];[3]atrim=duration=1[p2];[4]aresample=16000[s2];[s0][p1][s1][p2][s2]concat=n=5:v=0:a=1[out]" \
  -map "[out]" -ac 1 -c:a libvorbis -q:a 2 \
  testdata/sample.ogg
```

Generated with FFmpeg 8.0.

**Validation:**

```bash
# Should show 2 silence periods at ~2-3s and ~5-6s
ffmpeg -i testdata/sample.ogg -af "silencedetect=noise=-30dB:d=0.5" -f null - 2>&1 | grep silence
```

### short.ogg

Audio file (~3s) with NO detectable silence. Used to test `SilenceChunker` fallback to `TimeChunker` when no silences are found.

**Generation command:**

```bash
ffmpeg -f lavfi -i "sine=frequency=440:duration=3" \
       -c:a libvorbis -q:a 2 -ar 16000 -ac 1 \
       testdata/short.ogg
```

**Validation:**

```bash
# Should show NO silence_start or silence_end lines
ffmpeg -i testdata/short.ogg -af "silencedetect=noise=-30dB:d=0.5" -f null - 2>&1 | grep silence

# Should show duration ~3.0s
ffprobe -v quiet -show_entries format=duration testdata/short.ogg
```

## Text Fixtures

### silence_detect_output.txt

Sample FFmpeg silencedetect output (3 silences, 90s total duration). Used by `chunker_test.go` to test `parseSilenceOutput()` without running real FFmpeg.

### device_list_macos.txt

Sample macOS AVFoundation device list with BlackHole. Used by `loopback_test.go` to test device detection parsing.

### device_list_linux.txt

Sample Linux ALSA device list. Used by `loopback_test.go` to test device detection parsing.

## Programmatic Fixtures

### generateLongTranscript()

Long transcripts (>80K tokens) are generated programmatically via `generateLongTranscript(tokens int)` in `testhelpers_test.go` rather than stored as static files. This avoids ~300KB of repo bloat while providing flexible, deterministic content.

See `testhelpers_test.go` for implementation and `TestGenerateLongTranscript_*` for validation tests.

## Important Notes

- Audio fixture content is not semantically meaningful (synthetic signals)
- Only the structural properties matter (presence/absence of silence, duration)
- If a fixture is corrupted or lost, regenerate using the commands above
- All audio files use: OGG Vorbis, 16kHz, mono, quality 2 (~50kbps)
