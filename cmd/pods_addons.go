package cmd

import (
	"fmt"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

// Attaching an addon to a pod is what injects that addon's credentials into
// the pod's environment (DATABASE_URL, REDIS_URL, ...). Provisioning an addon
// on the machine is NOT enough on its own — an unattached addon is invisible
// to the pod, which is the usual reason a new pod cannot reach a database that
// visibly exists.
//
// The old spelling needed three UUIDs:
//   usectl apps addons attach 80f75010-... 83830047-... dc2f4fe2-...
// All three are now nameable:
//   usectl machines pods attach-addon runbyagents painboard database/painboard

var podsAttachAddonCmd = &cobra.Command{
	Use:     "attach-addon [machine] <pod> <addon>...",
	Aliases: []string{"attach"},
	Short:   "Attach addons to a pod so their credentials are injected as env vars",
	Long: `Attach one or more addons to a pod. Each addon's connection details are
injected into the pod as environment variables on the next rollout.

Machine, pod and addon may each be a name or a UUID. An addon can be named
by its instance name ("painboard"), its type ("database"), or "type/name"
when a bare name would be ambiguous.`,
	Example: `  usectl machines pods attach-addon api web database
  usectl machines pods attach-addon api web database/analytics redis
  usectl machines pods attach-addon api web --all`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID, podID, refs, err := resolveMachineAndPod(client, args)
		if err != nil {
			return err
		}
		var ids []string
		if podsAttachAll {
			all, aErr := client.ListProjectAddons(machineID)
			if aErr != nil {
				return aErr
			}
			for _, a := range all {
				ids = append(ids, a.ID)
			}
		}
		if len(refs) == 0 && !podsAttachAll {
			return fmt.Errorf("name at least one addon, or pass --all")
		}
		// Resolve every reference before attaching any, so a typo in the third
		// argument does not leave the first two already applied.
		for _, ref := range refs {
			id, rErr := resolveAddon(client, machineID, ref)
			if rErr != nil {
				return rErr
			}
			ids = append(ids, id)
		}

		for _, id := range ids {
			if err := client.AttachAppAddon(machineID, podID, id); err != nil {
				return fmt.Errorf("attach %s: %w", id, err)
			}
		}
		if jsonOutput {
			return output.JSON(map[string]any{"pod": podID, "attached": ids})
		}
		fmt.Printf("✓ Attached %d addon(s). Pod rolls with the new env vars.\n", len(ids))
		return nil
	},
}

var podsDetachAddonCmd = &cobra.Command{
	Use:     "detach-addon [machine] <pod> <addon>...",
	Aliases: []string{"detach"},
	Short:   "Detach addons from a pod (stop injecting their env vars)",
	Args:    cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID, podID, refs, err := resolveMachineAndPod(client, args)
		if err != nil {
			return err
		}
		var ids []string
		for _, ref := range refs {
			id, rErr := resolveAddon(client, machineID, ref)
			if rErr != nil {
				return rErr
			}
			ids = append(ids, id)
		}
		for _, id := range ids {
			if err := client.DetachAppAddon(machineID, podID, id); err != nil {
				return fmt.Errorf("detach %s: %w", id, err)
			}
		}
		fmt.Printf("✓ Detached %d addon(s). Pod rolls without those env vars.\n", len(ids))
		return nil
	},
}

var podsAddonsCmd = &cobra.Command{
	Use:   "addons [machine] <pod>",
	Short: "List the addons whose env vars are injected into a pod",
	Args:  cobra.RangeArgs(0, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID, podID, _, err := resolveMachineAndPod(client, args)
		if err != nil {
			return err
		}
		attached, err := client.ListAppAddonAttachments(machineID, podID)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(attached)
		}
		if len(attached) == 0 {
			fmt.Println("No addons attached — this pod receives no addon env vars.")
			fmt.Printf("\nAttach one:\n  usectl machines pods attach-addon %s %s <addon>\n", args[0], args[1])
			return nil
		}
		rows := make([][]string, len(attached))
		for i, a := range attached {
			rows[i] = []string{a.AddonType + "/" + a.Name, a.Mode, a.Status, envPrefixLabel(a)}
		}
		output.Table([]string{"ADDON", "MODE", "STATUS", "ENV PREFIX"}, rows)
		return nil
	},
}

// envPrefixLabel explains how the addon's variables are named inside the pod.
// A second instance is prefixed by its upper-cased name, so an "analytics"
// database appears as ANALYTICS_DATABASE_URL rather than DATABASE_URL.
func envPrefixLabel(a api.ProjectAddon) string {
	if a.EnvPrefix != "" {
		return a.EnvPrefix
	}
	if a.Name == "" || a.Name == "primary" {
		return "(none)"
	}
	return strings.ToUpper(a.Name) + "_"
}

var podsAttachAll bool

func init() {
	podsAttachAddonCmd.Flags().BoolVar(&podsAttachAll, "all", false, "Attach every addon in the machine")
}
