package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Manage API server contexts",
	Long:  "Manage multiple API server contexts (similar to kubectl contexts)",
}

var contextListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all contexts",
	RunE:  runContextList,
}

var contextCurrentCmd = &cobra.Command{
	Use:   "current",
	Short: "Display the current context",
	RunE:  runContextCurrent,
}

var contextUseCmd = &cobra.Command{
	Use:   "use <context-name>",
	Short: "Switch to a different context",
	Args:  cobra.ExactArgs(1),
	RunE:  runContextUse,
}

var contextDeleteCmd = &cobra.Command{
	Use:   "delete <context-name>",
	Short: "Delete a context",
	Args:  cobra.ExactArgs(1),
	RunE:  runContextDelete,
}

var contextRenameCmd = &cobra.Command{
	Use:   "rename <old-name> <new-name>",
	Short: "Rename a context",
	Args:  cobra.ExactArgs(2),
	RunE:  runContextRename,
}

func init() {
	contextCmd.AddCommand(contextListCmd)
	contextCmd.AddCommand(contextCurrentCmd)
	contextCmd.AddCommand(contextUseCmd)
	contextCmd.AddCommand(contextDeleteCmd)
	contextCmd.AddCommand(contextRenameCmd)
}

func runContextList(cmd *cobra.Command, args []string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if len(cfg.Contexts) == 0 {
		fmt.Println("No contexts configured. Run 'login' to create one.")
		return nil
	}

	fmt.Println("\nAvailable Contexts:")
	fmt.Println("===================")

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	_, _ = fmt.Fprintln(w, "CURRENT\tNAME\tAPI URL\tAUTHENTICATED")
	_, _ = fmt.Fprintln(w, "-------\t----\t-------\t-------------")

	for _, ctx := range cfg.Contexts {
		current := " "
		if ctx.Name == cfg.CurrentContext {
			current = "*"
		}

		authenticated := "No"
		if ctx.Token != "" {
			authenticated = "Yes"
		}

		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", current, ctx.Name, ctx.APIURL, authenticated)
	}

	_ = w.Flush()
	fmt.Println()

	return nil
}

func runContextCurrent(cmd *cobra.Command, args []string) error {
	ctx, err := GetCurrentContext()
	if err != nil {
		return err
	}

	fmt.Printf("Current context: %s\n", ctx.Name)
	fmt.Printf("API URL: %s\n", ctx.APIURL)
	if ctx.Token != "" {
		fmt.Println("Status: Authenticated")
	} else {
		fmt.Println("Status: Not authenticated (run 'login')")
	}

	return nil
}

func runContextUse(cmd *cobra.Command, args []string) error {
	contextName := args[0]

	if err := SetCurrentContext(contextName); err != nil {
		return err
	}

	fmt.Printf("✓ Switched to context '%s'\n", contextName)
	return nil
}

func runContextDelete(cmd *cobra.Command, args []string) error {
	contextName := args[0]

	if err := DeleteContext(contextName); err != nil {
		return err
	}

	fmt.Printf("✓ Deleted context '%s'\n", contextName)
	return nil
}

func runContextRename(cmd *cobra.Command, args []string) error {
	oldName := args[0]
	newName := args[1]

	if err := RenameContext(oldName, newName); err != nil {
		return err
	}

	fmt.Printf("✓ Renamed context '%s' to '%s'\n", oldName, newName)
	return nil
}

// NewContextCmd returns the context command
func NewContextCmd() *cobra.Command {
	return contextCmd
}
