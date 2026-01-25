package main

import (
	"fmt"
	"os"
	"slices"

	"github.com/spf13/cobra"
)

// validConfigKeys lists all supported configuration keys.
var validConfigKeys = []string{
	ConfigKeyOutputDir,
}

// configCmd creates the config command with subcommands.
func configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration settings",
		Long: `Manage persistent configuration settings.

Configuration is stored in ~/.config/go-transcript/config.
Settings can also be overridden via environment variables.

Supported settings:
  output-dir    Default directory for output files (env: TRANSCRIPT_OUTPUT_DIR)`,
		Example: `  transcript config set output-dir ~/Documents/transcripts
  transcript config get output-dir
  transcript config list`,
	}

	cmd.AddCommand(configSetCmd())
	cmd.AddCommand(configGetCmd())
	cmd.AddCommand(configListCmd())

	return cmd
}

// configSetCmd creates the "config set" subcommand.
func configSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Long: `Set a configuration value.

Supported keys:
  output-dir    Default directory for output files

The directory will be created if it doesn't exist.`,
		Example: `  transcript config set output-dir ~/Documents/transcripts
  transcript config set output-dir /tmp/recordings`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value := args[0], args[1]
			return runConfigSet(key, value)
		},
	}
}

// configGetCmd creates the "config get" subcommand.
func configGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Get a configuration value",
		Long: `Get a configuration value.

Prints the value to stdout, or nothing if not set.`,
		Example: `  transcript config get output-dir`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigGet(args[0])
		},
	}
}

// configListCmd creates the "config list" subcommand.
func configListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all configuration values",
		Long: `List all configuration values.

Shows both values from the config file and environment variable overrides.`,
		Example: `  transcript config list`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigList()
		},
	}
}

// runConfigSet handles the "config set" command.
func runConfigSet(key, value string) error {
	// Validate key.
	if !isValidConfigKey(key) {
		return fmt.Errorf("unknown config key %q (valid keys: %v)", key, validConfigKeys)
	}

	// Key-specific validation.
	switch key {
	case ConfigKeyOutputDir:
		// Expand ~ and validate directory.
		expanded := ExpandPath(value)
		if err := ValidOutputDir(expanded); err != nil {
			return fmt.Errorf("invalid output-dir: %w", err)
		}
		// Store the expanded path for consistency.
		value = expanded
	}

	// Save to config file.
	if err := SaveConfigValue(key, value); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Set %s = %s\n", key, value)
	return nil
}

// runConfigGet handles the "config get" command.
func runConfigGet(key string) error {
	// Validate key.
	if !isValidConfigKey(key) {
		return fmt.Errorf("unknown config key %q (valid keys: %v)", key, validConfigKeys)
	}

	value, err := GetConfigValue(key)
	if err != nil {
		return err
	}

	// Check environment variable fallback.
	if value == "" {
		switch key {
		case ConfigKeyOutputDir:
			value = os.Getenv(envOutputDir)
		}
	}

	if value != "" {
		fmt.Println(value)
	}

	return nil
}

// runConfigList handles the "config list" command.
func runConfigList() error {
	data, err := ListConfig()
	if err != nil {
		return err
	}

	// Add environment variable values for completeness.
	if _, ok := data[ConfigKeyOutputDir]; !ok {
		if env := os.Getenv(envOutputDir); env != "" {
			data[ConfigKeyOutputDir] = env + " (from env)"
		}
	}

	if len(data) == 0 {
		fmt.Println("No configuration set.")
		fmt.Println("\nAvailable settings:")
		for _, key := range validConfigKeys {
			fmt.Printf("  %s\n", key)
		}
		return nil
	}

	for key, value := range data {
		fmt.Printf("%s=%s\n", key, value)
	}

	return nil
}

// isValidConfigKey checks if a key is a valid configuration key.
func isValidConfigKey(key string) bool {
	return slices.Contains(validConfigKeys, key)
}
