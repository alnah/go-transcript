package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config keys.
const (
	ConfigKeyOutputDir = "output-dir"
)

// Environment variable fallbacks.
const (
	envOutputDir = "TRANSCRIPT_OUTPUT_DIR"
)

// Config holds user configuration loaded from ~/.config/go-transcript/config.
type Config struct {
	OutputDir string
}

// configDir returns the configuration directory path.
// Uses XDG_CONFIG_HOME if set, otherwise ~/.config/go-transcript.
func configDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "go-transcript"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "go-transcript"), nil
}

// configPath returns the full path to the config file.
func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config"), nil
}

// LoadConfig reads the configuration file and environment variables.
// Precedence: config file values, then environment variable fallbacks.
// Returns an empty Config if the file doesn't exist (not an error).
func LoadConfig() (Config, error) {
	var cfg Config

	path, err := configPath()
	if err != nil {
		return cfg, err
	}

	// Read config file if it exists.
	if data, err := parseConfigFile(path); err == nil {
		cfg.OutputDir = data[ConfigKeyOutputDir]
	} else if !os.IsNotExist(err) {
		return cfg, fmt.Errorf("failed to read config: %w", err)
	}

	// Environment variable fallback (only if not set in config).
	if cfg.OutputDir == "" {
		cfg.OutputDir = os.Getenv(envOutputDir)
	}

	return cfg, nil
}

// parseConfigFile reads a key=value config file.
// Format: one key=value per line, # comments, empty lines ignored.
func parseConfigFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

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

// SaveConfigValue writes a single key=value to the config file.
// Creates the config directory and file if they don't exist.
// Preserves existing values and comments.
func SaveConfigValue(key, value string) error {
	path, err := configPath()
	if err != nil {
		return err
	}

	// Ensure config directory exists.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create config directory: %w", err)
	}

	// Read existing config (if any).
	existing, _ := parseConfigFile(path)
	if existing == nil {
		existing = make(map[string]string)
	}

	// Update value.
	existing[key] = value

	// Write back.
	return writeConfigFile(path, existing)
}

// writeConfigFile writes the config map to a file.
func writeConfigFile(path string, data map[string]string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("cannot write config file: %w", err)
	}
	defer f.Close()

	for key, value := range data {
		if _, err := fmt.Fprintf(f, "%s=%s\n", key, value); err != nil {
			return fmt.Errorf("failed to write config: %w", err)
		}
	}

	return nil
}

// GetConfigValue reads a single value from the config file.
// Returns empty string if the key doesn't exist.
func GetConfigValue(key string) (string, error) {
	path, err := configPath()
	if err != nil {
		return "", err
	}

	data, err := parseConfigFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	return data[key], nil
}

// ListConfig returns all config values as a map.
func ListConfig() (map[string]string, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := parseConfigFile(path)
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
func ResolveOutputPath(output, outputDir, defaultName string) string {
	// Case 1: Explicit absolute path - use as-is.
	if output != "" && filepath.IsAbs(output) {
		return output
	}

	// Case 2: Explicit relative path - combine with outputDir if set.
	if output != "" {
		if outputDir != "" {
			return filepath.Join(outputDir, output)
		}
		return output
	}

	// Case 3: No output specified - use default name.
	if outputDir != "" {
		return filepath.Join(outputDir, defaultName)
	}
	return defaultName
}

// ValidOutputDir checks if a directory path is valid for use as output-dir.
// Returns nil if valid, or an error describing the problem.
func ValidOutputDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("output-dir cannot be empty")
	}

	// Expand ~ to home directory.
	if strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot expand ~: %w", err)
		}
		dir = filepath.Join(home, dir[2:])
	}

	// Check if path exists.
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			// Directory doesn't exist - try to create it.
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("cannot create directory: %w", err)
			}
			return nil
		}
		return fmt.Errorf("cannot access directory: %w", err)
	}

	// Check if it's a directory.
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", dir)
	}

	// Check if writable by attempting to create a temp file.
	testFile := filepath.Join(dir, ".go-transcript-write-test")
	f, err := os.Create(testFile)
	if err != nil {
		return fmt.Errorf("directory is not writable: %w", err)
	}
	f.Close()
	os.Remove(testFile)

	return nil
}

// ExpandPath expands ~ to the user's home directory.
func ExpandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
