package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
)

// usectl admin digest — the nightly ops digest.

var adminDigestCmd = &cobra.Command{
	Use:   "digest [day]",
	Short: "Show the nightly ops digest (latest, or a YYYY-MM-DD)",
	Long: `The ops digest is produced once a day by the API: failed deploys, quota
pressure, cert expiry, backup verification, DR watchdog, AI spend anomalies,
incidents, billing, email outbox and node health — with a short narrative and
the ranked items that need a human.

  usectl admin digest                 latest
  usectl admin digest 2026-09-18
  usectl admin digest list
  usectl admin digest run             produce today's digest now (also delivers it)`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		day := ""
		if len(args) == 1 {
			day = args[0]
		}
		d, err := client.GetOpsDigest(day)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(d)
		}
		printDigest(d)
		return nil
	},
}

func printDigest(d *api.OpsDigest) {
	if d.Status != "done" {
		fmt.Printf("Digest %s: %s", d.Day, d.Status)
		if d.Error != nil {
			fmt.Printf(" (%s)", *d.Error)
		}
		fmt.Println()
		return
	}
	fmt.Println(d.Rendered)
	fmt.Printf("\n[%s · %s · %.1fs]\n", strings.ToUpper(d.Severity), d.Model, float64(d.DurationMs)/1000)
}

var adminDigestListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent digests",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		limit, _ := cmd.Flags().GetInt("limit")
		list, err := client.ListOpsDigests(limit)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(list)
		}
		if len(list) == 0 {
			fmt.Println("No digests yet — run 'usectl admin digest run'.")
			return nil
		}
		rows := make([][]string, 0, len(list))
		for _, d := range list {
			rows = append(rows, []string{d.Day, d.Status, strings.ToUpper(d.Severity), d.Headline})
		}
		output.Table([]string{"DAY", "STATUS", "SEV", "HEADLINE"}, rows)
		return nil
	},
}

var adminDigestRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Produce today's digest now and deliver it",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		client.SetTimeout(5 * time.Minute)
		fmt.Fprintln(cmd.ErrOrStderr(), "Collecting and summarising (this can take a minute)…")
		d, err := client.RunOpsDigest()
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(d)
		}
		printDigest(d)
		return nil
	},
}

func init() {
	adminDigestListCmd.Flags().Int("limit", 30, "rows")
	adminDigestCmd.AddCommand(adminDigestListCmd, adminDigestRunCmd)
	adminCmd.AddCommand(adminDigestCmd)
}
