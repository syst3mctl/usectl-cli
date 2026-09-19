package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

// usectl alerts — built-in health checks, custom threshold rules and
// destinations, per machine (mig 088). Works on every machine; no Grafana
// addon involved. Incidents the rules raise live under `usectl incidents`.

var alertsGroup string

var alertsCmd = &cobra.Command{
	Use:     "alerts",
	Aliases: []string{"alert"},
	Short:   "Health checks, alert rules and destinations for a machine",
	Long: `Every machine has built-in health checks (crash loop, out of memory,
unavailable replicas, memory pressure) routed to the usectl agent, which turns
a firing check into an incident with proposed actions ('usectl incidents').
Add custom rules for traffic, latency, volumes, AI spend or your own PromQL,
and Slack/webhook destinations next to the agent.

Machines with groups: pass --group <name> to address one group's pods.`,
}

// alertsGroupID turns --group into the id the API expects ("" = ungrouped).
func alertsGroupID(client *api.Client, projectID string) (string, error) {
	if alertsGroup == "" {
		return "", nil
	}
	groups, err := client.ListProjectGroups(projectID)
	if err != nil {
		return "", err
	}
	for _, g := range groups {
		if strings.EqualFold(g.Name, alertsGroup) || g.ID == alertsGroup {
			return g.ID, nil
		}
	}
	return "", fmt.Errorf("no group named %q on this machine", alertsGroup)
}

var alertsListCmd = &cobra.Command{
	Use:   "list [machine]",
	Short: "Show health checks, rules and destinations",
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}
		gid, err := alertsGroupID(client, args[0])
		if err != nil {
			return err
		}
		checks, available, err := client.ListHealthChecks(args[0], gid)
		if err != nil {
			return err
		}
		rules, err := client.ListAlertRules(args[0], gid)
		if err != nil {
			return err
		}
		dests, err := client.ListAlertDestinations(args[0], gid)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(map[string]any{"health_checks": checks, "health_available": available, "rules": rules, "destinations": dests})
		}
		if available {
			fmt.Println("HEALTH CHECKS")
			for _, c := range checks {
				state := "on "
				if !c.Enabled {
					state = "off"
				}
				fmt.Printf("  %s  %-22s %s\n", state, c.Key, c.Description)
			}
			fmt.Println()
		}
		fmt.Println("CUSTOM RULES")
		n := 0
		for _, r := range rules {
			if r.Annotations["usectl_health"] != "" {
				continue
			}
			n++
			fmt.Printf("  %-16s %s  (for %s → %s)\n", r.UID, r.Title, r.For, r.NotificationSettings.Receiver)
		}
		if n == 0 {
			fmt.Println("  none — 'usectl alerts add' to create one")
		}
		fmt.Println()
		fmt.Println("DESTINATIONS")
		for _, d := range dests {
			if strings.Contains(d.Settings.URL, "/api/hooks/alerts/") || d.Name == "usectl agent" {
				fmt.Printf("  %-16s %-20s usectl agent — analyses the alert and proposes actions\n", d.UID, d.Name)
			} else {
				fmt.Printf("  %-16s %-20s %s %s\n", d.UID, d.Name, d.Type, d.Settings.URL)
			}
		}
		return nil
	},
}

var alertsHealthCmd = &cobra.Command{
	Use:   "health [machine] <check> <on|off>",
	Short: "Switch a built-in health check on or off (crashloop, oom, unavailable, memory)",
	Args:  cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}
		if len(args) != 3 {
			return fmt.Errorf("usage: usectl alerts health [machine] <check> <on|off>")
		}
		gid, err := alertsGroupID(client, args[0])
		if err != nil {
			return err
		}
		enabled := strings.EqualFold(args[2], "on") || strings.EqualFold(args[2], "true")
		checks, err := client.SetHealthCheck(args[0], gid, args[1], enabled)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(checks)
		}
		for _, c := range checks {
			if c.Key == args[1] {
				state := "on"
				if !c.Enabled {
					state = "off"
				}
				fmt.Printf("✓ %s is %s\n", c.Label, state)
			}
		}
		return nil
	},
}

var alertsMetricsCmd = &cobra.Command{
	Use:   "metrics [machine]",
	Short: "Metrics a custom rule can use",
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}
		metrics, err := client.ListAlertMetrics(args[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(metrics)
		}
		rows := [][]string{}
		for _, m := range metrics {
			rows = append(rows, []string{m.Value, m.Label, m.Unit, m.Targets, m.Description})
		}
		output.Table([]string{"METRIC", "LABEL", "UNIT", "FOR", "DESCRIPTION"}, rows)
		return nil
	},
}

var alertsAddCmd = &cobra.Command{
	Use:   "add [machine] <pod> <metric> <'>'|'<'> <threshold>",
	Short: "Add a custom threshold rule",
	Example: `  usectl alerts add web http_5xx_pct '>' 5 --for 5m
  usectl alerts add worker memory_pct '>' 90 --to team-slack
  usectl alerts add api custom '>' 100 --expr 'sum(rate(my_queue_depth[5m]))' --label "queue depth"`,
	Args: cobra.RangeArgs(4, 5),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}
		if len(args) != 5 {
			return fmt.Errorf("usage: usectl alerts add [machine] <pod> <metric> <'>'|'<'> <threshold>")
		}
		gid, err := alertsGroupID(client, args[0])
		if err != nil {
			return err
		}
		threshold, err := strconv.ParseFloat(args[4], 64)
		if err != nil {
			return fmt.Errorf("threshold must be a number")
		}
		targets, err := client.ListAlertTargets(args[0], gid)
		if err != nil {
			return err
		}
		var target *api.AlertTarget
		for i := range targets {
			if strings.EqualFold(targets[i].Name, args[1]) || targets[i].K8sName == args[1] {
				target = &targets[i]
			}
		}
		if target == nil {
			names := []string{}
			for _, t := range targets {
				names = append(names, t.Name)
			}
			return fmt.Errorf("no pod %q here — targets: %s", args[1], strings.Join(names, ", "))
		}
		forDur, _ := cmd.Flags().GetString("for")
		dest, _ := cmd.Flags().GetString("to")
		expr, _ := cmd.Flags().GetString("expr")
		label, _ := cmd.Flags().GetString("label")
		rule, err := client.CreateAlertRule(args[0], gid, api.CreateAlertRuleRequest{
			TargetType: target.Type, TargetName: target.Name, TargetK8s: target.K8sName,
			Metric: args[2], Expr: expr, Label: label, Comparator: args[3], Threshold: threshold,
			ForDuration: forDur, ContactPoint: dest,
		})
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(rule)
		}
		fmt.Printf("✓ %s (uid %s) → %s\n", rule.Title, rule.UID, dest)
		return nil
	},
}

var alertsRmCmd = &cobra.Command{
	Use:   "rm [machine] <rule-uid>",
	Short: "Delete a custom rule",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}
		if len(args) != 2 {
			return fmt.Errorf("usage: usectl alerts rm [machine] <rule-uid>")
		}
		gid, err := alertsGroupID(client, args[0])
		if err != nil {
			return err
		}
		if err := client.DeleteAlertRule(args[0], gid, args[1]); err != nil {
			return err
		}
		fmt.Println("✓ rule deleted")
		return nil
	},
}

var alertsDestAddCmd = &cobra.Command{
	Use:   "destination-add [machine] <name> <slack|webhook> <url>",
	Short: "Add a Slack or webhook destination",
	Args:  cobra.RangeArgs(3, 4),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}
		if len(args) != 4 {
			return fmt.Errorf("usage: usectl alerts destination-add [machine] <name> <slack|webhook> <url>")
		}
		gid, err := alertsGroupID(client, args[0])
		if err != nil {
			return err
		}
		d, err := client.CreateAlertDestination(args[0], gid, args[1], args[2], args[3])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(d)
		}
		fmt.Printf("✓ destination %s (uid %s)\n", d.Name, d.UID)
		return nil
	},
}

var alertsDestRmCmd = &cobra.Command{
	Use:   "destination-rm [machine] <uid>",
	Short: "Remove a Slack or webhook destination",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}
		if len(args) != 2 {
			return fmt.Errorf("usage: usectl alerts destination-rm [machine] <uid>")
		}
		gid, err := alertsGroupID(client, args[0])
		if err != nil {
			return err
		}
		if err := client.DeleteAlertDestination(args[0], gid, args[1]); err != nil {
			return err
		}
		fmt.Println("✓ destination removed")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(alertsCmd)
	alertsCmd.PersistentFlags().StringVar(&alertsGroup, "group", "", "machine group (environment) name; default = the ungrouped pods")
	alertsAddCmd.Flags().String("for", "5m", "how long the condition must hold (1m, 5m, 15m…)")
	alertsAddCmd.Flags().String("to", "usectl agent", "destination name")
	alertsAddCmd.Flags().String("expr", "", "PromQL for metric=custom (scoped to the machine by the platform)")
	alertsAddCmd.Flags().String("label", "", "display label for a custom expression")
	alertsCmd.AddCommand(alertsListCmd, alertsHealthCmd, alertsMetricsCmd, alertsAddCmd, alertsRmCmd, alertsDestAddCmd, alertsDestRmCmd)
}
