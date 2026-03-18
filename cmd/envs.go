package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

var envsCmd = &cobra.Command{
	Use:     "envs",
	Aliases: []string{"env"},
	Short:   "Manage custom environment variables for a project",
	Long: `Manage custom environment variables that are securely stored in an encrypted
vault and injected into your application at deploy time.

Variables are merged on update — existing variables not included in the
request are preserved. Changes take effect on the next deployment.`,
}

var envsListCmd = &cobra.Command{
	Use:   "list <project-id>",
	Short: "List all custom environment variables for a project",
	Example: `  usectl envs list a8f15889
  usectl envs list a8f15889 --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}

		envs, err := client.ListEnvs(args[0])
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(envs)
		}

		if len(envs) == 0 {
			fmt.Println("No custom environment variables set.")
			fmt.Println("\n  Hint: usectl envs set <project-id> KEY=value")
			return nil
		}

		// Sort keys for consistent output.
		keys := make([]string, 0, len(envs))
		for k := range envs {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		rows := make([][]string, len(keys))
		for i, k := range keys {
			v := envs[k]
			// Mask long values.
			if len(v) > 40 {
				v = v[:20] + "..." + v[len(v)-8:]
			}
			rows[i] = []string{k, v}
		}
		output.Table([]string{"KEY", "VALUE"}, rows)
		return nil
	},
}

var envsSetCmd = &cobra.Command{
	Use:   "set <project-id> KEY=value [KEY=value ...]",
	Short: "Set or update environment variables (merge behavior)",
	Long: `Set one or more environment variables for a project. Uses merge behavior —
existing variables not included in this command are preserved.

Changes take effect on the next deployment. Trigger a deploy with:
  usectl projects deploy <project-id>`,
	Example: `  usectl envs set a8f15889 API_KEY=sk-123 NODE_ENV=production
  usectl envs set a8f15889 STRIPE_SECRET=sk_live_abc123`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}

		projectID := args[0]
		vars := make(map[string]string)
		for _, arg := range args[1:] {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid format: %q (expected KEY=value)", arg)
			}
			vars[parts[0]] = parts[1]
		}

		if err := client.SetEnvs(projectID, vars); err != nil {
			return err
		}

		fmt.Printf("✓ Set %d environment variable(s). Deploy to apply:\n", len(vars))
		fmt.Printf("  usectl projects deploy %s\n", projectID)
		return nil
	},
}

var envsDeleteCmd = &cobra.Command{
	Use:     "delete <project-id> KEY [KEY ...]",
	Aliases: []string{"rm", "remove", "unset"},
	Short:   "Delete specific environment variables",
	Example: `  usectl envs delete a8f15889 DEBUG OLD_VAR
  usectl envs unset a8f15889 API_KEY`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}

		projectID := args[0]
		keys := args[1:]

		if err := client.DeleteEnvs(projectID, keys); err != nil {
			return err
		}

		fmt.Printf("✓ Deleted %d environment variable(s). Deploy to apply:\n", len(keys))
		fmt.Printf("  usectl projects deploy %s\n", projectID)
		return nil
	},
}

func init() {
	envsCmd.AddCommand(envsListCmd)
	envsCmd.AddCommand(envsSetCmd)
	envsCmd.AddCommand(envsDeleteCmd)
	rootCmd.AddCommand(envsCmd)
}
