package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/dmwork-org/octo-daemon-cli/internal"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the daemon",
	Long:  "Start detecting local agent runtimes and reporting to Octo server.",
	RunE:  runStart,
}

var (
	flagAPIKey     string
	flagAPIURL     string
	flagDeviceName string
	flagForeground bool
)

func init() {
	startCmd.Flags().StringVar(&flagAPIKey, "api-key", "", "User API key for authentication (required)")
	startCmd.Flags().StringVar(&flagAPIURL, "api-url", "", "Octo API server URL (required)")
	startCmd.Flags().StringVar(&flagDeviceName, "device-name", "", "Device display name (defaults to hostname)")
	startCmd.Flags().BoolVar(&flagForeground, "foreground", true, "Run in foreground (default: true)")

	startCmd.MarkFlagRequired("api-key")
	startCmd.MarkFlagRequired("api-url")
}

func runStart(cmd *cobra.Command, args []string) error {
	deviceName := flagDeviceName
	if deviceName == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("get hostname: %w", err)
		}
		deviceName = hostname
	}

	cfg := internal.Config{
		APIKey:     flagAPIKey,
		APIURL:     flagAPIURL,
		DeviceName: deviceName,
		CLIVersion: version,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	d, err := internal.NewDaemon(cfg)
	if err != nil {
		return fmt.Errorf("init daemon: %w", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Run(ctx)
	}()

	select {
	case sig := <-sigCh:
		fmt.Printf("\nReceived %s, shutting down...\n", sig)
		cancel()
		return <-errCh
	case err := <-errCh:
		return err
	}
}
