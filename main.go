package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/zijiren233/sealos-state-metric/app"
	"github.com/zijiren233/sealos-state-metric/pkg/config"
	"github.com/zijiren233/sealos-state-metric/pkg/logger"
)

func main() {
	// Parse command-line flags
	opts := app.NewOptions()
	opts.AddFlags(flag.CommandLine)
	flag.Parse()

	// Load configuration
	loader := config.NewConfigLoader(opts.ConfigFile, opts.EnvFile)

	cfg, err := loader.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Override config with command-line flags
	if opts.MetricsAddress != "" {
		cfg.Server.Address = opts.MetricsAddress
	}

	if opts.KubeconfigFile != "" {
		cfg.Kubernetes.Kubeconfig = opts.KubeconfigFile
	}

	if opts.EnabledCollectors != "" {
		cfg.EnabledCollectors = opts.ParsedEnabledCollectors()
	}

	cfg.LeaderElection.Enabled = opts.LeaderElectionMode

	// Override logging level
	if opts.LogLevel != "" {
		cfg.Logging.Level = opts.LogLevel
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Configuration validation failed: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logger.InitLog(
		logger.WithDebug(cfg.Logging.Debug),
		logger.WithLevel(cfg.Logging.Level),
		logger.WithFormat(cfg.Logging.Format),
	)

	log.WithFields(log.Fields{
		"collectors":     cfg.EnabledCollectors,
		"leaderElection": cfg.LeaderElection.Enabled,
		"metricsAddress": cfg.Server.Address,
	}).Info("Configuration loaded")

	// Create and run server
	ctx := context.Background()

	server, err := app.NewServer(cfg, opts.ConfigFile)
	if err != nil {
		log.WithError(err).Error("Failed to create server")
		os.Exit(1)
	}

	if err := server.Run(ctx); err != nil {
		log.WithError(err).Error("Server exited with error")
		os.Exit(1)
	}

	log.Info("Server exited successfully")
}
