package main

import (
	"fmt"
	"os"

	"github.com/davidcohan/port-authorizing/internal/cli"
	"github.com/davidcohan/port-authorizing/internal/server"
	"github.com/spf13/cobra"
)

var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "port-authorizing",
		Short: "Secure database access proxy with authentication and authorization",
		Long: `Port Authorizing provides secure, time-limited access to databases and services
with centralized authentication, role-based authorization, and query whitelisting.`,
		Version: fmt.Sprintf("%s (built %s, commit %s)", Version, BuildTime, GitCommit),
	}

	// Server command
	serverCmd := &cobra.Command{
		Use:   "server",
		Short: "Start the API server",
		Long:  "Start the Port Authorizing API server that handles authentication and proxying",
		RunE:  server.RunServer,
	}
	serverCmd.Flags().String("config", "config.yaml", "Path to configuration file")

	// Client commands (login, list, connect)
	loginCmd := cli.NewLoginCmd()
	listCmd := cli.NewListCmd()
	connectCmd := cli.NewConnectCmd()

	// Version command
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Port Authorizing %s\n", Version)
			fmt.Printf("Build Time: %s\n", BuildTime)
			fmt.Printf("Git Commit: %s\n", GitCommit)
		},
	}

	// Add commands
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(connectCmd)
	rootCmd.AddCommand(versionCmd)

	// Global flags
	rootCmd.PersistentFlags().String("api-url", "http://localhost:8080", "API server URL")
	rootCmd.PersistentFlags().String("config", os.Getenv("HOME")+"/.port-auth/config.json", "Path to CLI config file")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
