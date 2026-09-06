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
	Use:     "envs [machine] [pod] [KEY=value ...]",
	Aliases: []string{"env", "variables", "vars"},
	Short:   "Show or set environment variables for a machine or one pod",
	Long: `Environment variables, stored in an encrypted vault and injected at deploy
time. Updates merge: keys you do not mention are preserved.

Naming a pod shows that POD's full environment — machine-wide values, its own
overrides, and the credentials of every attached addon, each tagged with where
it came from. Without a pod, only the machine-wide variables are shown.

  usectl machines envs api                 machine-wide
  usectl machines envs api web             everything the pod 'web' receives
  usectl machines envs api KEY=value       set machine-wide
  usectl machines envs api web KEY=value   set on that pod only

Machine and pod may be given in either order. The explicit subcommands
(list / set / delete / protect) still work unchanged.`,
	Example: `  usectl machines envs api
  usectl machines envs api web
  usectl machines envs api LOG_LEVEL=debug`,
	Args: cobra.ArbitraryArgs,
	// Runnable so that naming a machine does something useful. As a bare
	// command group it printed help and silently ignored its arguments,
	// which read as "envs are not listable" rather than "you missed a
	// subcommand".
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			// No target at all: fall back to the context, else show help.
			if _, _, err := machineRef(""); err != nil {
				return cmd.Help()
			}
		}
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}

		var positional, pairs []string
		for _, a := range args {
			if strings.Contains(a, "=") {
				pairs = append(pairs, a)
			} else {
				positional = append(positional, a)
			}
		}

		machineID, podID, err := resolveMachineOptionalPod(client, positional)
		if err != nil {
			return err
		}

		if len(pairs) > 0 {
			vars := map[string]string{}
			for _, p := range pairs {
				k, v, _ := strings.Cut(p, "=")
				vars[k] = v
			}
			if podID != "" {
				if err := client.UpdateAppEnvs(machineID, podID, vars); err != nil {
					return err
				}
				fmt.Printf("✓ Set %d variable(s) on this pod. Takes effect on the next deploy.\n", len(vars))
				return nil
			}
			if err := client.SetEnvs(machineID, vars); err != nil {
				return err
			}
			fmt.Printf("✓ Set %d machine-wide variable(s). Takes effect on the next deploy.\n", len(vars))
			return nil
		}

		if podID != "" {
			return printPodEnv(client, machineID, podID)
		}
		return printMachineEnv(client, machineID)
	},
}

var envsListCmd = &cobra.Command{
	Use:   "list [machine]",
	Short: "List all custom environment variables for a machine",
	Example: `  usectl envs list a8f15889
  usectl envs list a8f15889 --json`,
	Args: cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
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
			fmt.Println("\n  Hint: usectl envs set [machine] KEY=value")
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
			ptr := envs[k]
			// A nil value means the variable is protected: the API refuses
			// to return it to any client. Print a placeholder — never a
			// partial preview, since this output lands in terminals and CI
			// logs (USCT-183).
			if ptr == nil {
				rows[i] = []string{k, "(protected)"}
				continue
			}
			v := *ptr
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
	Use:   "set [machine] KEY=value [KEY=value ...]",
	Short: "Set or update environment variables (merge behavior)",
	Long: `Set one or more environment variables for a machine. Uses merge behavior —
existing variables not included in this command are preserved.

Changes take effect on the next deployment. Trigger a deploy with:
  usectl machines deploy [machine]`,
	Example: `  usectl envs set a8f15889 API_KEY=sk-123 NODE_ENV=production
  usectl envs set a8f15889 STRIPE_SECRET=sk_live_abc123`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
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
		fmt.Printf("  usectl machines deploy %s\n", projectID)
		return nil
	},
}

var envsDeleteCmd = &cobra.Command{
	Use:     "delete [machine] KEY [KEY ...]",
	Aliases: []string{"rm", "remove", "unset"},
	Short:   "Delete specific environment variables",
	Example: `  usectl envs delete a8f15889 DEBUG OLD_VAR
  usectl envs unset a8f15889 API_KEY`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}

		projectID := args[0]
		keys := args[1:]

		if err := client.DeleteEnvs(projectID, keys); err != nil {
			return err
		}

		fmt.Printf("✓ Deleted %d environment variable(s). Deploy to apply:\n", len(keys))
		fmt.Printf("  usectl machines deploy %s\n", projectID)
		return nil
	},
}

// protectionArg maps the user's word to the boolean the API wants. Spelled as
// an explicit `protect` / `open` verb pair rather than a --protected=false
// flag so that un-protecting a credential is always a deliberate, readable
// command in shell history and CI scripts.
func protectionArg(mode string) (bool, error) {
	switch mode {
	case "protect", "protected":
		return true, nil
	case "open", "unprotect":
		return false, nil
	}
	return false, fmt.Errorf("unknown mode %q (expected \"protect\" or \"open\")", mode)
}

var envsProtectCmd = &cobra.Command{
	Use:   "protect [machine] [pod] <protect|open> KEY [KEY ...]",
	Short: "Mark environment variables protected (write-only) or open",
	Long: `Control whether a variable's value can be read back.

A protected variable is write-only: its value is never returned by the
dashboard, the API, or this CLI — only replaced. Deployments are unaffected,
the real value still reaches your pods.

Open variables are returned in full to anyone who can already read the
machine's variables.`,
	Example: `  usectl machines envs protect api protect STRIPE_SECRET_KEY DB_PASSWORD
  usectl machines envs protect api open LOG_LEVEL
  usectl machines envs protect api web protect API_KEY   # this pod's override`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}

		// The mode word splits the machine/pod prefix from the key list, so
		// it is found by position-independent scan rather than a fixed index.
		// That also lets an optional pod sit before it.
		modeAt := -1
		for i, a := range args {
			if m := strings.ToLower(a); m == "protect" || m == "open" {
				modeAt = i
				break
			}
		}
		if modeAt < 0 {
			return fmt.Errorf("expected 'protect' or 'open' before the key list")
		}
		keys := args[modeAt+1:]
		if len(keys) == 0 {
			return fmt.Errorf("name at least one variable to mark")
		}
		protected, err := protectionArg(strings.ToLower(args[modeAt]))
		if err != nil {
			return err
		}

		// Resolve the target. Without this a machine NAME — or a UUID with a
		// character missing — went straight to the API as a project id and
		// came back "invalid project id" tagged with the variable name.
		machineID, podID, err := resolveMachineOptionalPod(client, args[:modeAt])
		if err != nil {
			return err
		}

		for _, key := range keys {
			var pErr error
			if podID != "" {
				pErr = client.SetAppVarProtection(machineID, podID, key, protected)
			} else {
				pErr = client.SetVarProtection(machineID, key, protected)
			}
			if pErr != nil {
				return fmt.Errorf("%s: %w", key, pErr)
			}
		}
		args = keys
		state := "protected (write-only)"
		if !protected {
			state = "open"
		}
		fmt.Printf("✓ Marked %d variable(s) %s\n", len(args), state)
		return nil
	},
}

func init() {
	envsCmd.AddCommand(envsListCmd)
	envsCmd.AddCommand(envsSetCmd)
	envsCmd.AddCommand(envsDeleteCmd)
	envsCmd.AddCommand(envsProtectCmd)
	rootCmd.AddCommand(envsCmd)
}
