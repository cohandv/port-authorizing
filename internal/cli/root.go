package cli

import (
	"github.com/spf13/cobra"
)

var (
	apiURL     string
	configPath string
)

var rootCmd = &cobra.Command{
	Use:   "port-authorizing-cli",
	Short: "CLI client for port-authorizing proxy system",
	Long: `A CLI client that connects to the port-authorizing API server
to establish authenticated proxy connections to protected services.`,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&apiURL, "api-url", "http://localhost:8080", "API server URL")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "$HOME/.port-auth/config.json", "Path to CLI config file")

	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(connectCmd)
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}
