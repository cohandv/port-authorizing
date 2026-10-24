package cli

import (
	"github.com/spf13/cobra"
)

var (
	apiURL     string
	configPath string
	rootCmd    *cobra.Command
)

// Execute builds and runs the root command for the standalone CLI binary.
// Built lazily here (rather than in an init()) so that the combined
// port-authorizing binary - which builds its own separate root command in
// cmd/port-authorizing/main.go and never calls this - doesn't pay for a
// redundant LoadConfig()+binary-resolution pass on every invocation.
func Execute() error {
	rootCmd = &cobra.Command{
		Use:   "port-authorizing",
		Short: "Port Authorizing - Secure proxy for any service",
		Long: `Port Authorizing provides secure, authenticated, and audited access to any service.
It acts as a transparent proxy with role-based access control, query/request whitelisting,
approval workflows, and comprehensive audit logging.`,
	}

	// Add persistent flags
	rootCmd.PersistentFlags().StringVar(&apiURL, "api-url", "http://localhost:8080", "API server URL")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "Path to config file")

	// Add subcommands
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(connectCmd)
	rootCmd.AddCommand(contextCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(NewClientBindingCommands()...)

	return rootCmd.Execute()
}

// NewLoginCmd returns the login command
func NewLoginCmd() *cobra.Command {
	return loginCmd
}

// NewListCmd returns the list command
func NewListCmd() *cobra.Command {
	return listCmd
}

// NewConnectCmd returns the connect command
func NewConnectCmd() *cobra.Command {
	return connectCmd
}

// NewVersionCmd returns the version command
func NewVersionCmd() *cobra.Command {
	return versionCmd
}
