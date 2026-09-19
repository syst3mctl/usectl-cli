package cmd

import (
	"fmt"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

// Weekly machine digest (mig 085): one message per machine per week to its
// owner and developers — deploys, incidents, health, quota, AI, changes.

var (
	digestWeek    string
	digestRun     bool
	digestDeliver bool
	digestList    bool
)

var machineDigestCmd = &cobra.Command{
	Use:   "digest [machine]",
	Short: "Read the machine's weekly digest, or generate it now",
	Long: `The weekly digest goes out every Monday 07:00 UTC to the machine's owner and
developers (Inbox, email, push). This prints the latest one.

  usectl machines digest api                 latest digest
  usectl machines digest api --week 2026-09-15   a given week (its Monday)
  usectl machines digest api --list          weeks available
  usectl machines digest api --run           generate this week so far (preview, not sent)
  usectl machines digest api --run --deliver ...and send it to the recipients`,
	Args: cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}
		m := args[0]
		if digestList {
			list, err := client.ListMachineDigests(m, 12)
			if err != nil {
				return err
			}
			if jsonOutput {
				return output.JSON(list)
			}
			if len(list) == 0 {
				fmt.Println("No digests yet — the first one arrives next Monday, or run with --run.")
				return nil
			}
			rows := make([][]string, 0, len(list))
			for _, d := range list {
				rows = append(rows, []string{d.WeekStart, d.Status, d.Severity, d.Headline})
			}
			output.Table([]string{"WEEK", "STATUS", "SEVERITY", "HEADLINE"}, rows)
			return nil
		}
		var d *api.MachineDigest
		if digestRun {
			d, err = client.RunMachineDigest(m, digestDeliver)
		} else {
			week := digestWeek
			if week == "" {
				week = "latest"
			}
			d, err = client.GetMachineDigest(m, week)
		}
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(d)
		}
		if d.Status == "failed" && d.Error != nil {
			return fmt.Errorf("digest failed: %s", *d.Error)
		}
		fmt.Println(strings.TrimSpace(d.Rendered))
		if digestRun && digestDeliver {
			fmt.Println("\n✓ Delivered to the owner and developers.")
		}
		return nil
	},
}

var machineDigestSettingsCmd = &cobra.Command{
	Use:   "settings [machine]",
	Short: "Show or change the weekly digest settings",
	Example: `  usectl machines digest settings api
  usectl machines digest settings api --enabled off
  usectl machines digest settings api --skip-quiet off
  usectl machines digest settings api --email ops@example.com,cto@example.com
  usectl machines digest settings api --email none`,
	Args: cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}
		body := map[string]any{}
		onoff := func(flag string) (*bool, error) {
			if !cmd.Flags().Changed(flag) {
				return nil, nil
			}
			v, _ := cmd.Flags().GetString(flag)
			switch strings.ToLower(v) {
			case "on", "true", "yes":
				b := true
				return &b, nil
			case "off", "false", "no":
				b := false
				return &b, nil
			}
			return nil, fmt.Errorf("--%s must be on or off", flag)
		}
		if b, err := onoff("enabled"); err != nil {
			return err
		} else if b != nil {
			body["enabled"] = *b
		}
		if b, err := onoff("skip-quiet"); err != nil {
			return err
		} else if b != nil {
			body["skip_quiet"] = *b
		}
		if cmd.Flags().Changed("email") {
			v, _ := cmd.Flags().GetString("email")
			if strings.EqualFold(v, "none") {
				body["extra_emails"] = []string{}
			} else {
				body["extra_emails"] = strings.Split(v, ",")
			}
		}
		var s *api.MachineDigestSettings
		if len(body) > 0 {
			s, err = client.PutMachineDigestSettings(args[0], body)
		} else {
			s, err = client.GetMachineDigestSettings(args[0])
		}
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(s)
		}
		fmt.Printf("Weekly digest  %v\nSkip quiet     %v\nExtra emails   %s\n", s.Enabled, s.SkipQuiet, strings.Join(s.ExtraEmails, ", "))
		fmt.Println("Recipients: the machine owner and developer members (Inbox, email, push), plus the extra emails.")
		return nil
	},
}

func init() {
	machineDigestCmd.Flags().StringVar(&digestWeek, "week", "", "Week to show (its Monday, YYYY-MM-DD); default latest")
	machineDigestCmd.Flags().BoolVar(&digestList, "list", false, "List available weeks")
	machineDigestCmd.Flags().BoolVar(&digestRun, "run", false, "Generate this week's digest now (preview)")
	machineDigestCmd.Flags().BoolVar(&digestDeliver, "deliver", false, "With --run: also send it")
	machineDigestSettingsCmd.Flags().String("enabled", "", "on | off")
	machineDigestSettingsCmd.Flags().String("skip-quiet", "", "on | off — skip weeks where nothing happened")
	machineDigestSettingsCmd.Flags().String("email", "", "Extra recipients, comma-separated (none to clear)")
	machineDigestCmd.AddCommand(machineDigestSettingsCmd)
	projectsCmd.AddCommand(machineDigestCmd)
}
