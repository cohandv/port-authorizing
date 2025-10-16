package server

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/davidcohan/port-authorizing/internal/api"
	"github.com/davidcohan/port-authorizing/internal/config"
	"github.com/spf13/cobra"
)

func RunServer(cmd *cobra.Command, args []string) error {
	configPath, _ := cmd.Flags().GetString("config")

	// Parse command line flags (for backward compatibility)
	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	configFlag := fs.String("config", configPath, "Path to configuration file")
	_ = fs.Parse(args)

	if *configFlag != "" {
		configPath = *configFlag
	}

	// Load configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create and start server
	server, err := api.NewServer(cfg)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Load the latest configuration from storage backend if configured
	if cfg.Storage != nil {
		log.Println("Loading latest configuration from storage backend...")
		latestCfg, err := server.LoadConfigFromStorage()
		if err != nil {
			log.Printf("Warning: Failed to load config from storage backend: %v", err)
			log.Println("Using initial configuration from file")
		} else {
			log.Println("Successfully loaded configuration from storage backend")
			// Reload the server with the latest config
			if err := server.ReloadConfig(latestCfg); err != nil {
				log.Printf("Warning: Failed to reload config: %v", err)
			}
		}
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down gracefully...")
		if err := server.Shutdown(); err != nil {
			log.Printf("Error during shutdown: %v", err)
		}
		os.Exit(0)
	}()

	// Start server
	log.Printf("Starting API server on port %d", cfg.Server.Port)
	if err := server.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}

	return nil
}
