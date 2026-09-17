package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
)

// usectl incidents — what the alert → agent path produced for a machine, and
// the policy that decides whether the agent may act on its own.

var incidentsCmd = &cobra.Command{
	Use:     "incidents",
	Aliases: []string{"incident"},
	Short:   "Incidents raised by alerts, with the agent's summary and proposed actions",
	Long: `When an alert rule notifies the "usectl agent" contact point, the agent reads
logs, metrics and recent deployments, writes a summary and proposes actions.

  usectl incidents list [machine]                     newest first
  usectl incidents get [machine] <incident-id>        full detail + actions
  usectl incidents approve [machine] <incident-id> <action-id>
  usectl incidents reject  [machine] <incident-id> <action-id>
  usectl incidents policy  [machine] [--mode notify|approve|auto ...]`,
}

var incidentsListCmd = &cobra.Command{
	Use:   "list [machine]",
	Short: "List a machine's incidents, newest first",
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}
		status, _ := cmd.Flags().GetString("status")
		limit, _ := cmd.Flags().GetInt("limit")
		list, err := client.ListIncidents(args[0], status, limit)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(list)
		}
		if len(list) == 0 {
			fmt.Println("No incidents.")
			return nil
		}
		rows := make([][]string, 0, len(list))
		for _, in := range list {
			pending := 0
			for _, a := range in.Actions {
				if a.Status == "proposed" || a.Status == "auto_blocked" {
					pending++
				}
			}
			head := in.Headline
			if head == "" {
				head = in.AlertName
			}
			note := ""
			if pending > 0 {
				note = fmt.Sprintf("%d action(s) awaiting approval", pending)
			}
			rows = append(rows, []string{in.ID, in.Status, strings.ToUpper(in.Severity), head, humanAge(in.StartedAt.Format("2006-01-02T15:04:05Z07:00")), note})
		}
		output.Table([]string{"ID", "STATUS", "SEV", "HEADLINE", "STARTED", "NOTE"}, rows)
		return nil
	},
}

var incidentsGetCmd = &cobra.Command{
	Use:   "get [machine] <incident-id>",
	Short: "Show an incident's summary, evidence and actions",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}
		in, err := client.GetIncident(args[0], args[1])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(in)
		}
		printIncident(in)
		return nil
	},
}

func printIncident(in *api.Incident) {
	fmt.Printf("[%s] %s\n", strings.ToUpper(in.Severity), in.Headline)
	fmt.Printf("alert: %s   status: %s   started: %s", in.AlertName, in.Status, in.StartedAt.Local().Format("2006-01-02 15:04"))
	if in.ResolvedAt != nil {
		fmt.Printf("   resolved: %s", in.ResolvedAt.Local().Format("15:04"))
	}
	fmt.Println()
	if in.Status != "open" && in.Status != "resolved" {
		if in.Error != nil {
			fmt.Printf("(%s)\n", *in.Error)
		}
		return
	}
	fmt.Printf("confidence: %.0f%%   model: %s   policy: %s\n\n%s\n", in.Confidence*100, in.Model, in.PolicyMode, in.Summary)
	if in.ProbableCause != "" {
		fmt.Printf("\nProbable cause: %s\n", in.ProbableCause)
	}
	if in.CorrelatedDeploymentID != nil {
		fmt.Printf("Correlated deployment: %s\n", *in.CorrelatedDeploymentID)
	}
	if len(in.Evidence) > 0 {
		fmt.Println("\nEvidence:")
		for _, e := range in.Evidence {
			fmt.Printf("  %s\n", e)
		}
	}
	if len(in.Actions) > 0 {
		fmt.Println("\nActions:")
		for _, a := range in.Actions {
			fmt.Printf("  %d. [%s] %s %v — %s (risk %s)\n     id %s", a.Seq, a.Status, a.Tool, a.Args, a.Why, a.Risk, a.ID)
			if a.Result != nil && *a.Result != "" {
				fmt.Printf("\n     %s", strings.SplitN(*a.Result, "\n", 2)[0])
			}
			fmt.Println()
		}
	}
}

func incidentActionCmd(verb string) *cobra.Command {
	short := "Approve and execute a proposed action (runs as you)"
	if verb == "reject" {
		short = "Reject a proposed action"
	}
	return &cobra.Command{
		Use:   verb + " [machine] <incident-id> <action-id>",
		Short: short,
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := api.NewClient(apiURL)
			if err != nil {
				return err
			}
			if args, err = resolveFirstArg(client, args); err != nil {
				return err
			}
			var a *api.IncidentAction
			if verb == "approve" {
				a, err = client.ApproveIncidentAction(args[0], args[1], args[2])
			} else {
				a, err = client.RejectIncidentAction(args[0], args[1], args[2])
			}
			if err != nil {
				return err
			}
			if jsonOutput {
				return output.JSON(a)
			}
			fmt.Printf("%s: %s → %s\n", a.Tool, verb, a.Status)
			if a.Result != nil && *a.Result != "" {
				fmt.Println(*a.Result)
			}
			return nil
		},
	}
}

var incidentsPolicyCmd = &cobra.Command{
	Use:   "policy [machine]",
	Short: "Show or change whether the agent may act on its own",
	Long: `Modes: notify (summarise only, default), approve (actions wait for a human),
auto (the agent executes allowed actions itself when every guard passes:
confidence, allowlist, daily budget, cooldown, and for rollback a correlated
deployment under an hour old with an intact previous image).`,
	Args: cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}
		patch := map[string]interface{}{}
		if v, _ := cmd.Flags().GetString("mode"); v != "" {
			patch["mode"] = v
		}
		if v, _ := cmd.Flags().GetString("allow"); v != "" {
			patch["allowed_tools"] = strings.Split(v, ",")
		}
		if cmd.Flags().Changed("min-confidence") {
			v, _ := cmd.Flags().GetFloat64("min-confidence")
			patch["min_confidence"] = v
		}
		if cmd.Flags().Changed("max-per-day") {
			v, _ := cmd.Flags().GetInt("max-per-day")
			patch["max_auto_per_day"] = v
		}
		if cmd.Flags().Changed("cooldown") {
			v, _ := cmd.Flags().GetInt("cooldown")
			patch["cooldown_minutes"] = v
		}
		var p *api.IncidentPolicy
		if len(patch) > 0 {
			p, err = client.PutIncidentPolicy(args[0], patch)
		} else {
			p, err = client.GetIncidentPolicy(args[0])
		}
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(p)
		}
		fmt.Printf("mode: %s\nauto allowlist: %s\nmin confidence: %.0f%%\nmax auto/day: %d\ncooldown: %d min\n",
			p.Mode, strings.Join(p.AllowedTools, ", "), p.MinConfidence*100, p.MaxAutoPerDay, p.CooldownMinutes)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(incidentsCmd)
	incidentsListCmd.Flags().String("status", "", "filter: queued, running, open, resolved, skipped, failed")
	incidentsListCmd.Flags().Int("limit", 20, "max rows")
	incidentsPolicyCmd.Flags().String("mode", "", "notify | approve | auto")
	incidentsPolicyCmd.Flags().String("allow", "", "comma-separated auto allowlist: rollback,restart_pod,resize_pod,delete_kpod")
	incidentsPolicyCmd.Flags().Float64("min-confidence", 0.8, "minimum confidence for auto actions (0.5-1)")
	incidentsPolicyCmd.Flags().Int("max-per-day", 3, "auto actions per pod per day")
	incidentsPolicyCmd.Flags().Int("cooldown", 120, "minutes between auto actions on one pod")
	incidentsCmd.AddCommand(incidentsListCmd, incidentsGetCmd, incidentActionCmd("approve"), incidentActionCmd("reject"), incidentsPolicyCmd)
}
