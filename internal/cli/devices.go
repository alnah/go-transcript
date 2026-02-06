package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

// DevicesCmd creates the devices command.
// Lists available audio input devices for use with --device.
func DevicesCmd(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:   "devices",
		Short: "List available audio input devices",
		Long: `List available audio input devices detected by FFmpeg.

Use the device name with --device in the record or live commands.
Devices are sorted with real microphones first, virtual devices last.`,
		Example: `  transcript devices
  transcript record -d 30m --device "MacBook Pro Microphone"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runListDevices(cmd.Context(), env)
		},
	}
}

// runListDevices resolves FFmpeg and lists available audio devices.
func runListDevices(ctx context.Context, env *Env) error {
	ffmpegPath, err := env.FFmpegResolver.Resolve(ctx)
	if err != nil {
		return err
	}

	lister, err := env.DeviceListerFactory.NewDeviceLister(ffmpegPath)
	if err != nil {
		return err
	}

	devices, err := lister.ListDevices(ctx)
	if err != nil {
		return err
	}

	if len(devices) == 0 {
		fmt.Fprintln(env.Stderr, "No audio input devices found.")
		return nil
	}

	for _, d := range devices {
		fmt.Fprintln(env.Stderr, d)
	}
	return nil
}
