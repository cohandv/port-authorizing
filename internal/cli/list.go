package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available connections",
	Long:  "Display all available proxy connections configured on the API server",
	RunE:  runList,
}

type connectionInfo struct {
	Name     string            `json:"name"`
	Type     string            `json:"type"`
	Tags     []string          `json:"tags,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

func runList(cmd *cobra.Command, args []string) error {
	// Get context (--context if passed, else the current context)
	ctx, err := resolveContext(cmd)
	if err != nil {
		return fmt.Errorf("not logged in: %w. Please run 'login' first", err)
	}

	apiURL := resolveAPIURL(cmd, ctx.Name, ctx.APIURL)
	token := ctx.Token

	connections, err := fetchConnections(apiURL, token)
	if err != nil {
		return err
	}

	// Display connections
	fmt.Println("\nAvailable Connections:")
	fmt.Println("----------------------")
	for _, conn := range connections {
		fmt.Printf("  • %s [%s]", conn.Name, conn.Type)
		
		// Show tags inline if present
		if len(conn.Tags) > 0 {
			fmt.Printf(" tags: %s", formatTags(conn.Tags))
		}
		fmt.Println()
		
		// Show metadata as indented details
		if len(conn.Metadata) > 0 {
			for key, value := range conn.Metadata {
				fmt.Printf("    %s: %s\n", key, value)
			}
		}
	}
	fmt.Println()

	return nil
}

func formatTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	result := ""
	for i, tag := range tags {
		if i > 0 {
			result += ", "
		}
		result += tag
	}
	return result
}
