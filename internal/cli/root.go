package cli

import (
	"github.com/spf13/cobra"
)

var (
	apiURL     string
	configPath string
)

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
