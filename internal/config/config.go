package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config keys.
const (
	KeyOutputDir = "output-dir"
)

// Environment variable fallbacks.
const (
	EnvOutputDir = "TRANSCRIPT_OUTPUT_DIR"
)

// Config holds user configuration loaded from ~/.config/go-transcript/config.
type Config struct {
	OutputDir string
}

// dir returns the configuration directory path.
// Uses XDG_CONFIG_HOME if set, otherwise ~/.config/go-transcript.
func dir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "go-transcript"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "go-transcript"), nil
}

// path returns the full path to the config file.
func path() (string, error) {
	d, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config"), nil
}

// Load reads the configuration file and environment variables.
// Precedence: config file values, then environment variable fallbacks.
// Returns an empty Config if the file doesn't exist (not an error).
func Load() (Config, error) {
	var cfg Config

	p, err := path()
	if err != nil {
		return cfg, err
	}

	// Read config file if it exists.
	if data, err := parseFile(p); err == nil {
		cfg.OutputDir = data[KeyOutputDir]
	} else if !os.IsNotExist(err) {
		return cfg, fmt.Errorf("failed to read config: %w", err)
	}

	// Environment variable fallback (only if not set in config).
	if cfg.OutputDir == "" {
		cfg.OutputDir = os.Getenv(EnvOutputDir)
	}

	return cfg, nil
}

// parseFile reads a key=value config file.
// Format: one key=value per line, # comments, empty lines ignored.
func parseFile(p string) (map[string]string, error) {
	f, err := os.Open(p) // #nosec G304 -- config path is constructed from home dir
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	data := make(map[string]string)
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse key=value.
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid syntax at line %d: %q", lineNum, line)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		data[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	return data, nil
}

// Save writes a single key=value to the config file.
// Creates the config directory and file if they don't exist.
// Preserves existing key=value pairs but discards comments.
func Save(key, value string) error {
	p, err := path()
	if err != nil {
		return err
	}

	// Ensure config directory exists.
	d := filepath.Dir(p)
	if err := os.MkdirAll(d, 0750); err != nil { // #nosec G301 -- user config dir
		return fmt.Errorf("cannot create config directory: %w", err)
	}

	// Read existing config (if any).
	existing, _ := parseFile(p)
	if existing == nil {
		existing = make(map[string]string)
	}

	// Update value.
	existing[key] = value

	// Write back.
	return writeFile(p, existing)
}

// writeFile writes the config map to a file.
func writeFile(p string, data map[string]string) error {
	// #nosec G302 G304 -- config file with standard permissions, path from home dir
	f, err := os.OpenFile(p, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("cannot write config file: %w", err)
	}
	defer func() { _ = f.Close() }()

	for key, value := range data {
		if _, err := fmt.Fprintf(f, "%s=%s\n", key, value); err != nil {
			return fmt.Errorf("failed to write config: %w", err)
		}
	}

	return nil
}

// Get reads a single value from the config file.
// Returns empty string if the key doesn't exist.
func Get(key string) (string, error) {
	p, err := path()
	if err != nil {
		return "", err
	}

	data, err := parseFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	return data[key], nil
}

// List returns all config values as a map.
func List() (map[string]string, error) {
	p, err := path()
	if err != nil {
		return nil, err
	}

	data, err := parseFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil
		}
		return nil, err
	}

	return data, nil
}

// ResolveOutputPath resolves the final output path using the following precedence:
//  1. If output is absolute, use it as-is
//  2. If output is relative and outputDir is set, join them
//  3. If output is empty, use defaultName in outputDir (or cwd if no outputDir)
//
// outputDir can come from config or flag.
// All paths are cleaned using filepath.Clean to normalize separators and remove redundant elements.
func ResolveOutputPath(output, outputDir, defaultName string) string {
	// Case 1: Explicit absolute path - use as-is.
	if output != "" && filepath.IsAbs(output) {
		return filepath.Clean(output)
	}

	// Case 2: Explicit relative path - combine with outputDir if set.
	if output != "" {
		if outputDir != "" {
			return filepath.Clean(filepath.Join(outputDir, output))
		}
		return filepath.Clean(output)
	}

	// Case 3: No output specified - use default name.
	if outputDir != "" {
		return filepath.Clean(filepath.Join(outputDir, defaultName))
	}
	return filepath.Clean(defaultName)
}

// ValidOutputDir checks if a directory path is valid for use as output-dir.
// Returns nil if valid, or an error describing the problem.
func ValidOutputDir(d string) error {
	if d == "" {
		return fmt.Errorf("output-dir cannot be empty")
	}

	// Expand ~ to home directory.
	if strings.HasPrefix(d, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot expand ~: %w", err)
		}
		d = filepath.Join(home, d[2:])
	}

	// Check if path exists.
	info, err := os.Stat(d)
	if err != nil {
		if os.IsNotExist(err) {
			// Directory doesn't exist - try to create it.
			if err := os.MkdirAll(d, 0750); err != nil { // #nosec G301 -- user output dir
				return fmt.Errorf("cannot create directory: %w", err)
			}
			return nil
		}
		return fmt.Errorf("cannot access directory: %w", err)
	}

	// Check if it's a directory.
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", d)
	}

	// Check if writable by attempting to create a temp file.
	testFile := filepath.Join(d, ".go-transcript-write-test")
	f, err := os.Create(testFile) // #nosec G304 -- path is constructed from validated dir
	if err != nil {
		return fmt.Errorf("directory is not writable: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(testFile)
		return fmt.Errorf("directory is not writable: %w", err)
	}
	_ = os.Remove(testFile) // Best effort cleanup, ignore error

	return nil
}

// ExpandPath expands ~ to the user's home directory.
func ExpandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, p[2:])
	}
	return p
}

// Dir returns the configuration directory path (exported for testing).
func Dir() (string, error) {
	return dir()
}

// ParseFile reads a key=value config file (exported for testing).
func ParseFile(p string) (map[string]string, error) {
	return parseFile(p)
}
