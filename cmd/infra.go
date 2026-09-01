package cmd

import (
	"fmt"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

// Namespace-level pods, registry usage, and machine groups.
//
// All three were reachable from the dashboard but from no `usectl` command,
// so scripted and agent-driven workflows had to fall back to kubectl or curl.

var (
	podDeleteYes bool
	groupColor   string
	groupSort    int
	groupYes     bool
)

// ── kpods ─────────────────────────────────────────────────────────────

var kpodsCmd = &cobra.Command{
	Use:     "kpods",
	Aliases: []string{"k8s-pods"},
	Short:   "Raw Kubernetes pods across every namespace a machine owns",
	Long: `The Kubernetes view of a machine, including addon pods, group namespaces,
and pods that belong to no app.

Different from 'machines pods', which shows the app-level stats view. Use this
when you need the actual pod names — to read a crash reason, see which node
something landed on, or delete one pod without restarting the whole machine.`,
}

var kpodsListCmd = &cobra.Command{
	Use:   "list <machine-id>",
	Short: "List every pod in the machine's namespaces",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		pods, err := client.ListNamespacePods(args[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(pods)
		}
		if len(pods) == 0 {
			fmt.Println("No pods found.")
			return nil
		}
		rows := make([][]string, 0, len(pods))
		for _, p := range pods {
			phase := p.Phase
			if p.Terminating {
				// A terminating pod still reports Phase=Running, which reads
				// as healthy when it is on its way out.
				phase = "Terminating"
			}
			group := p.GroupName
			if group == "" {
				group = "—"
			}
			rows = append(rows, []string{
				p.Name, phase,
				fmt.Sprintf("%d/%d", p.Ready, p.Total),
				fmt.Sprint(p.Restarts),
				group, p.NodeName, p.Reason,
			})
		}
		output.Table([]string{"NAME", "PHASE", "READY", "RESTARTS", "GROUP", "NODE", "REASON"}, rows)
		return nil
	},
}

var kpodsDeleteCmd = &cobra.Command{
	Use:     "delete <machine-id> <pod-name>",
	Aliases: []string{"restart"},
	Short:   "Delete one pod so its controller recreates it",
	Long: `Deletes a single pod. The Deployment or StatefulSet that owns it creates a
replacement, so this is effectively "restart just this one".

Narrower than 'machines pods restart', which rolls every app in the machine.
Reach for this when one replica is wedged and the rest are healthy.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID, podName := args[0], args[1]
		if !podDeleteYes && !jsonOutput {
			fmt.Printf("Delete pod %s? Its controller will recreate it. [y/N] ", podName)
			var answer string
			_, _ = fmt.Scanln(&answer)
			if !strings.EqualFold(strings.TrimSpace(answer), "y") {
				fmt.Println("Aborted.")
				return nil
			}
		}
		if err := client.DeleteNamespacePod(machineID, podName); err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(map[string]string{"status": "deleted", "pod": podName})
		}
		fmt.Printf("Deleted %s — the controller will recreate it.\n", podName)
		return nil
	},
}

// ── registry usage ────────────────────────────────────────────────────

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Inspect the machine's container-image allowance",
}

var registryUsageCmd = &cobra.Command{
	Use:   "usage <machine-id>",
	Short: "Show registry usage against the machine's allowance",
	Long: `How much of the machine's image allowance is used, and by which images.

The allowance is separate from the plan's storage_gb: images live in the shared
platform registry, not in the machine's volumes.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		u, err := client.GetRegistryUsage(args[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(u)
		}
		fmt.Printf("Used:      %s\n", humanBytes(u.UsedBytes))
		fmt.Printf("Allowance: %s\n", humanBytes(u.AllowanceBytes))
		fmt.Printf("Free:      %s\n", humanBytes(u.FreeBytes))
		if len(u.Images) > 0 {
			fmt.Println()
			rows := make([][]string, len(u.Images))
			for i, img := range u.Images {
				rows[i] = []string{img.ImageRef, humanBytes(img.SizeBytes)}
			}
			output.Table([]string{"IMAGE", "SIZE"}, rows)
		}
		return nil
	},
}

// ── machine groups ────────────────────────────────────────────────────

var groupsCmd = &cobra.Command{
	Use:     "groups",
	Aliases: []string{"group"},
	Short:   "Partition a machine's apps and addons into isolated namespaces",
	Long: `A group is a sibling namespace, kdeploy-<machine>-<group>, with a NetworkPolicy
that blocks traffic to and from every other group — including the ungrouped
namespace. Ungrouped resources stay where they are; no migration is needed.

Groups share the machine's single ResourceQuota, so any one group can consume
the whole plan.

Renaming is not supported: a group is a Kubernetes namespace, and namespaces
cannot be renamed. Delete it, recreate it, and reassign its members.`,
}

var groupsListCmd = &cobra.Command{
	Use:   "list <machine-id>",
	Short: "List a machine's groups",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		groups, err := client.ListProjectGroups(args[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(groups)
		}
		if len(groups) == 0 {
			fmt.Println("No groups. All resources are in the machine's default namespace.")
			return nil
		}
		rows := make([][]string, len(groups))
		for i, g := range groups {
			color := "—"
			if g.Color != nil && *g.Color != "" {
				color = *g.Color
			}
			rows[i] = []string{g.ID, g.Name, color, fmt.Sprint(g.SortOrder)}
		}
		output.Table([]string{"ID", "NAME", "COLOR", "ORDER"}, rows)
		return nil
	},
}

var groupsCreateCmd = &cobra.Command{
	Use:   "create <machine-id> <name>",
	Short: "Create a group",
	Long: `Creates a group and its namespace.

The name becomes part of a Kubernetes namespace, so it is lowercased and must
be DNS-safe.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		var sort *int
		if cmd.Flags().Changed("sort") {
			sort = &groupSort
		}
		g, err := client.CreateProjectGroup(args[0], args[1], groupColor, sort)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(g)
		}
		fmt.Printf("Created group %s (%s)\n", g.Name, g.ID)
		return nil
	},
}

var groupsDeleteCmd = &cobra.Command{
	Use:   "delete <machine-id> <group-id>",
	Short: "Delete a group",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if !groupYes && !jsonOutput {
			fmt.Printf("Delete group %s? Its members must be reassigned first. [y/N] ", args[1])
			var answer string
			_, _ = fmt.Scanln(&answer)
			if !strings.EqualFold(strings.TrimSpace(answer), "y") {
				fmt.Println("Aborted.")
				return nil
			}
		}
		if err := client.DeleteProjectGroup(args[0], args[1]); err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(map[string]string{"status": "deleted", "group_id": args[1]})
		}
		fmt.Println("Group deleted.")
		return nil
	},
}

func init() {
	kpodsDeleteCmd.Flags().BoolVarP(&podDeleteYes, "yes", "y", false, "Skip the confirmation prompt")
	kpodsCmd.AddCommand(kpodsListCmd, kpodsDeleteCmd)

	registryCmd.AddCommand(registryUsageCmd)

	groupsCreateCmd.Flags().StringVar(&groupColor, "color", "", "Display colour (hex, e.g. #4f46e5)")
	groupsCreateCmd.Flags().IntVar(&groupSort, "sort", 0, "Sort order in the dashboard")
	groupsDeleteCmd.Flags().BoolVarP(&groupYes, "yes", "y", false, "Skip the confirmation prompt")
	groupsCmd.AddCommand(groupsListCmd, groupsCreateCmd, groupsDeleteCmd)

	rootCmd.AddCommand(kpodsCmd, registryCmd, groupsCmd)
}
