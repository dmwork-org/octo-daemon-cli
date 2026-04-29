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
	flagConfigFile string
)

func init() {
	startCmd.Flags().StringVar(&flagAPIKey, "api-key", "", "User API key for authentication")
	startCmd.Flags().StringVar(&flagAPIURL, "api-url", "", "Octo API server URL")
	startCmd.Flags().StringVar(&flagDeviceName, "device-name", "", "Device display name (defaults to hostname)")
	startCmd.Flags().BoolVar(&flagForeground, "foreground", true, "Run in foreground (default: true)")
	startCmd.Flags().StringVar(&flagConfigFile, "config", "", "Config file path (overrides api-key/api-url flags)")
}

func runStart(cmd *cobra.Command, args []string) error {
	var cfg internal.Config

	// 如果有 --config，从文件加载
	if flagConfigFile != "" {
		loaded, err := internal.LoadConfig(flagConfigFile)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		cfg = loaded
	}

	// 命令行参数覆盖配置文件
	if flagAPIKey != "" {
		cfg.APIKey = flagAPIKey
	}
	if flagAPIURL != "" {
		cfg.APIURL = flagAPIURL
	}
	if flagDeviceName != "" {
		cfg.DeviceName = flagDeviceName
	}

	if cfg.APIKey == "" || cfg.APIURL == "" {
		return fmt.Errorf("api-key and api-url are required (via flags or --config)")
	}

	if cfg.DeviceName == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("get hostname: %w", err)
		}
		cfg.DeviceName = hostname
	}

	cfg.CLIVersion = version

	// 持久化配置（升级后新进程用）
	if err := internal.SaveConfig(cfg); err != nil {
		fmt.Printf("[WARN] failed to save config: %v\n", err)
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
