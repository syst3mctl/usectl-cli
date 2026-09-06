package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

// Deployment history and rollback.
//
// The API has always exposed these; the CLI could cancel a deployment and
// fetch its logs but had no way to LIST deployments — so there was no way to
// discover the id those commands need without opening the dashboard.

var (
	deploymentsStatus  string
	deploymentsApp     string
	deploymentsPage    int
	deploymentsPerPage int
	rollbackReason     string
	rollbackYes        bool
)

var deploymentsCmd = &cobra.Command{
	Use:     "deployments",
	Aliases: []string{"deploys", "deployment"},
	Short:   "List deployment history and roll back to a previous build",
	Long: `Deployment history for a machine, newest first.

Every row carries the id that 'machines deploy-logs' and 'machines cancel'
need, plus the image that was actually deployed — which is what a rollback
redeploys.`,
}

var deploymentsListCmd = &cobra.Command{
	Use:   "list [machine]",
	Short: "List a machine's deployments, newest first",
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}
		page, err := client.ListDeployments(args[0], deploymentsStatus, deploymentsApp, deploymentsPage, deploymentsPerPage)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(page)
		}
		if len(page.Deployments) == 0 {
			fmt.Println("No deployments found.")
			return nil
		}
		rows := make([][]string, 0, len(page.Deployments))
		for _, d := range page.Deployments {
			commit := d.CommitHash
			if len(commit) > 7 {
				commit = commit[:7]
			}
			if commit == "" {
				// Image-sourced deployments have no commit at all (mig 065).
				// An em dash says "not applicable" rather than "missing".
				commit = "—"
			}
			src := d.SourceType
			if src == "" {
				src = "git"
			}
			note := d.RollbackState
			if note == "none" || note == "" {
				note = ""
			}
			if d.ImagePrunedAt != nil {
				// Worth surfacing in the list: this row can never be rolled
				// back to, and finding that out at the click is the failure
				// USCT-191 set out to remove.
				note = "image reclaimed"
			}
			if d.UpstreamCode != nil && *d.UpstreamCode != "" {
				note = *d.UpstreamCode
			}
			rows = append(rows, []string{
				d.ID, d.Status, src, commit,
				humanAge(d.CreatedAt), note,
			})
		}
		output.Table([]string{"ID", "STATUS", "SOURCE", "COMMIT", "AGE", "NOTE"}, rows)
		fmt.Printf("\nPage %d of %d  (%d total)\n", page.Page, page.TotalPages, page.Total)
		return nil
	},
}

var deploymentsRollbackCmd = &cobra.Command{
	Use:   "rollback [machine] <app-id> <deployment-id>",
	Short: "Redeploy the image a previous deployment ran",
	Long: `Rolls one app back to the image a previous deployment ran.

Scoped to an app, not the machine: each app has its own image, so there is no
single image a machine-wide rollback could mean.

The image is read from the stored deployment record rather than recomputed, so
what redeploys is the build that actually ran. If retention has reclaimed that
image the API refuses with 410 — 'deployments list' shows those rows as
"image reclaimed".`,
	Args: cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}
		machineID, appID, deploymentID := args[0], args[1], args[2]

		if !rollbackYes && !jsonOutput {
			fmt.Printf("Roll app %s back to deployment %s? [y/N] ", appID, deploymentID)
			var answer string
			_, _ = fmt.Scanln(&answer)
			if !strings.EqualFold(strings.TrimSpace(answer), "y") {
				fmt.Println("Aborted.")
				return nil
			}
		}

		if err := client.RollbackToDeployment(machineID, appID, deploymentID, rollbackReason); err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(map[string]string{
				"status": "rollback_started", "app_id": appID, "deployment_id": deploymentID,
			})
		}
		fmt.Printf("Rollback started for app %s → deployment %s\n", appID, deploymentID)
		fmt.Println("Track it with: usectl deployments list " + machineID)
		return nil
	},
}

// humanAge renders an RFC3339 timestamp as a compact relative age.
//
// Takes a string because the API's deployment timestamps are carried as
// strings on the existing type; an unparseable value renders as an em dash
// rather than a misleading "0s".
func humanAge(iso string) string {
	if iso == "" {
		return "—"
	}
	ts, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return "—"
	}
	d := time.Since(ts)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func init() {
	deploymentsListCmd.Flags().StringVar(&deploymentsStatus, "status", "", "Filter by status (live, building, failed, cancelled)")
	deploymentsListCmd.Flags().StringVar(&deploymentsApp, "app", "", "Filter to one app id")
	deploymentsListCmd.Flags().IntVar(&deploymentsPage, "page", 0, "Page number (default 1)")
	deploymentsListCmd.Flags().IntVar(&deploymentsPerPage, "per-page", 0, "Results per page (max 100)")

	deploymentsRollbackCmd.Flags().StringVar(&rollbackReason, "reason", "", "Reason recorded on the rollback")
	deploymentsRollbackCmd.Flags().BoolVarP(&rollbackYes, "yes", "y", false, "Skip the confirmation prompt")

	deploymentsCmd.AddCommand(deploymentsListCmd, deploymentsRollbackCmd)
	rootCmd.AddCommand(deploymentsCmd)
}
