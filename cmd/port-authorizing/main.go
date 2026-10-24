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

	// Client commands (login, list, connect, context, version)
	loginCmd := cli.NewLoginCmd()
	listCmd := cli.NewListCmd()
	connectCmd := cli.NewConnectCmd()
	contextCmd := cli.NewContextCmd()
	versionCmd := cli.NewVersionCmd()

	// Set version info in CLI package
	cli.SetVersion(Version, BuildTime, GitCommit)

	// Add commands
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(connectCmd)
	rootCmd.AddCommand(contextCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(cli.NewClientBindingCommands()...)

	// Global flags
	rootCmd.PersistentFlags().String("api-url", "", "API server URL (default: https://fwd.{context}.cohandv.io; override with a custom URL, optionally containing \"{env}\" to interpolate the context name)")
	rootCmd.PersistentFlags().StringP("context", "c", "", "Context to use for this command (default: current context). For 'list'/'connect' this only affects this invocation; for 'login' it's the context that gets saved and made current.")
	rootCmd.PersistentFlags().String("config", os.Getenv("HOME")+"/.port-auth/config.json", "Path to CLI config file")

	// This binary's "--config" flag is local to its own rootCmd, not bound
	// to the cli package's internal configPath - thread it through once
	// flags are parsed (and before any subcommand runs) so overriding it
	// actually affects which config file GetConfigPath()/LoadConfig() use.
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if path, _ := cmd.Flags().GetString("config"); path != "" {
			cli.SetConfigPath(path)
		}
		return nil
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
