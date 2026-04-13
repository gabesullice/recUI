package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/gabesullice/recui/pkg/config"
	"github.com/gabesullice/recui/pkg/recfile"
	"github.com/gabesullice/recui/pkg/server"
)

func main() {
	root := &cobra.Command{
		Use:           "recui",
		Short:         "Browser UI for recfiles",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	var flagPort int
	var flagWebDir string
	var flagConfig string

	serve := &cobra.Command{
		Use:   "serve <recfile>",
		Short: "Start the browser UI for a recfile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Configure slog before anything else logs.
			var handler slog.Handler
			if term.IsTerminal(int(os.Stderr.Fd())) {
				handler = slog.NewTextHandler(os.Stderr, nil)
			} else {
				handler = slog.NewJSONHandler(os.Stderr, nil)
			}
			slog.SetDefault(slog.New(handler))

			// Resolve and load config if --config was provided.
			var uiConfig config.UIConfig
			if flagConfig != "" {
				cfgPath, err := filepath.EvalSymlinks(flagConfig)
				if err != nil {
					fmt.Fprintf(os.Stderr, "recui: cannot resolve config path %q: %v\n", flagConfig, err)
					os.Exit(1)
				}
				uiConfig, err = config.LoadConfig(cfgPath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "recui: %v\n", err)
					os.Exit(1)
				}
			}

			// Canonicalize the path.
			canonicalPath, err := filepath.EvalSymlinks(args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "recui: cannot resolve path %q: %v\n", args[0], err)
				os.Exit(1)
			}

			// Wire a cancellable context to SIGINT/SIGTERM. server.Run observes
			// ctx and calls http.Server.Shutdown when it fires; the returned
			// error is nil on a clean drain.
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			cfg := server.Config{
				Addr:        "127.0.0.1",
				Port:        flagPort,
				RecfilePath: canonicalPath,
				WebDir:      flagWebDir,
				UIConfig:    uiConfig,
			}

			// onReady runs exactly once, inside server.Run, after the recfile
			// has been parsed — so the startup log reflects the authoritative
			// parse result without main.go performing a second ParseFile.
			onReady := func(types []recfile.RecordType) {
				// Warn on config type names not found in the recfile.
				for typeName := range uiConfig {
					found := false
					for _, rt := range types {
						if rt.Name == typeName {
							found = true
							break
						}
					}
					if !found {
						slog.Warn("config references unknown record type", "type", typeName)
					}
				}
				// Count total records across all types.
				totalRecords := 0
				for _, rt := range types {
					totalRecords += len(rt.Records)
				}
				addr := fmt.Sprintf("http://%s:%d", "127.0.0.1", flagPort)
				slog.Info("recui ready",
					"addr", addr,
					"recfile", canonicalPath,
					"types", len(types),
					"records", totalRecords,
				)
			}

			if runErr := server.Run(ctx, cfg, onReady); runErr != nil {
				// Check for port-already-in-use.
				var opErr *net.OpError
				if errors.As(runErr, &opErr) {
					if errors.Is(opErr.Err, syscall.EADDRINUSE) {
						fmt.Fprintf(os.Stderr, "port %d is already in use — use --port to specify a different port\n", flagPort)
						os.Exit(1)
					}
				}
				fmt.Fprintf(os.Stderr, "recui: server error: %v\n", runErr)
				os.Exit(1)
			}
			return nil
		},
	}

	serve.Flags().IntVarP(&flagPort, "port", "p", 8080, "TCP port to listen on")
	serve.Flags().StringVarP(&flagWebDir, "web-dir", "w", "", "serve static assets from this disk path instead of embedded FS")
	serve.Flags().StringVarP(&flagConfig, "config", "c", "", "path to TOML display config file")

	var flagOutputDir string
	var flagGenConfig string

	generate := &cobra.Command{
		Use:   "generate <recfile>",
		Short: "Generate a static website from a recfile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve and load config if --config was provided.
			var uiConfig config.UIConfig
			if flagGenConfig != "" {
				cfgPath, err := filepath.EvalSymlinks(flagGenConfig)
				if err != nil {
					fmt.Fprintf(os.Stderr, "recui: cannot resolve config path %q: %v\n", flagGenConfig, err)
					os.Exit(1)
				}
				var loadErr error
				uiConfig, loadErr = config.LoadConfig(cfgPath)
				if loadErr != nil {
					fmt.Fprintf(os.Stderr, "recui: %v\n", loadErr)
					os.Exit(1)
				}
			}
			// Canonicalize the recfile path.
			canonicalPath, err := filepath.EvalSymlinks(args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "recui: cannot resolve path %q: %v\n", args[0], err)
				os.Exit(1)
			}
			cfg := server.GenerateConfig{
				RecfilePath: canonicalPath,
				UIConfig:    uiConfig,
				OutputDir:   flagOutputDir,
			}
			if err := server.Generate(cfg); err != nil {
				fmt.Fprintf(os.Stderr, "recui: %v\n", err)
				os.Exit(1)
			}
			return nil
		},
	}

	generate.Flags().StringVarP(&flagOutputDir, "output", "o", "site", "output directory for the generated site")
	generate.Flags().StringVarP(&flagGenConfig, "config", "c", "", "path to TOML display config file")

	root.AddCommand(serve)
	root.AddCommand(generate)

	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "recui: %v\n", err)
		os.Exit(1)
	}
}
