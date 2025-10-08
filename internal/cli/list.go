package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

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
	Metadata map[string]string `json:"metadata,omitempty"`
}

func runList(cmd *cobra.Command, args []string) error {
	// Get current context
	ctx, err := GetCurrentContext()
	if err != nil {
		return fmt.Errorf("not logged in: %w. Please run 'login' first", err)
	}

	apiURL := ctx.APIURL
	token := ctx.Token

	// Allow override from command line flag
	if flagURL, _ := cmd.Root().PersistentFlags().GetString("api-url"); flagURL != "" {
		apiURL = flagURL
	}

	// Create request
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/connections", apiURL), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request failed: %s", string(body))
	}

	// Parse response
	var connections []connectionInfo
	if err := json.Unmarshal(body, &connections); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Display connections
	fmt.Println("\nAvailable Connections:")
	fmt.Println("----------------------")
	for _, conn := range connections {
		fmt.Printf("  • %s [%s]\n", conn.Name, conn.Type)
		if len(conn.Metadata) > 0 {
			for key, value := range conn.Metadata {
				fmt.Printf("    %s: %s\n", key, value)
			}
		}
	}
	fmt.Println()

	return nil
}
