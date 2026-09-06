package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

// ---- Stack detection ----

var (
	stackInstallID int64
	stackOwner     string
	stackRepo      string
	stackRef       string
)

var stackDetectCmd = &cobra.Command{
	Use:   "stack-detect",
	Short: "Detect the language/framework/conventions for a (repo, ref)",
	Long: `Returns the predicted language, framework, package manager, and other
conventions for a GitHub repository at a given ref. Used by the dashboard
new-app modal to preselect Dockerfile defaults.`,
	Example: `  usectl stack-detect --installation-id 1234 --owner me --repo my-app --ref main`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		s, err := client.DetectGitHubStack(stackInstallID, stackOwner, stackRepo, stackRef)
		if err != nil {
			return err
		}
		return output.JSON(s)
	},
}

// ---- Project billing ----

var billingProjectCmd = &cobra.Command{
	Use:     "project-billing",
	Aliases: []string{"pbilling"},
	Short:   "View per-machine billing details and open the machine's billing portal",
}

var billingProjectGetCmd = &cobra.Command{
	Use:   "get [machine]",
	Short: "Show billing status, plan, and resource allocation for a machine",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		b, err := client.GetProjectBilling(args[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(b)
		}
		output.Table([]string{"FIELD", "VALUE"}, [][]string{
			{"Status", b.BillingStatus},
			{"Interval", b.BillingInterval},
			{"Tier", b.ResourceTier},
			{"vCPU", strconv.FormatFloat(b.VCPU, 'f', 2, 64)},
			{"RAM (GiB)", strconv.FormatFloat(b.RAMGB, 'f', 2, 64)},
			{"Storage (GiB)", strconv.FormatFloat(b.StorageGB, 'f', 2, 64)},
			{"Monthly (cents)", strconv.Itoa(b.MonthlyPriceCents)},
		})
		return nil
	},
}

var (
	billingProjPortalReturn string
)

var billingProjectPortalCmd = &cobra.Command{
	Use:   "portal [machine]",
	Short: "Open the Stripe billing portal for a machine",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		url, err := client.CreateProjectBillingPortal(args[0], billingProjPortalReturn)
		if err != nil {
			return err
		}
		fmt.Println(url)
		return nil
	},
}

// ---- Pricing calculator ----

var (
	priceVCPU      float64
	priceRAMGB     float64
	priceStorageGB float64
	priceInterval  string
)

var priceCmd = &cobra.Command{
	Use:   "price",
	Short: "Calculate the monthly price for a given resource configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		calc, err := client.CalculatePrice(priceVCPU, priceRAMGB, priceStorageGB, priceInterval)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(calc)
		}
		fmt.Printf("Interval: %s\n", calc.Interval)
		fmt.Printf("Interval price: $%.2f\n", float64(calc.IntervalAmount)/100)
		fmt.Printf("Monthly total:  $%.2f\n", float64(calc.MonthlyTotal)/100)
		return nil
	},
}

// ---- Project domains list (vs the user's domain inventory) ----

var projectDomainsListCmd = &cobra.Command{
	Use:   "project-domains [machine]",
	Short: "List all domains attached to a single machine",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		domains, err := client.GetProjectDomains(args[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(domains)
		}
		if len(domains) == 0 {
			fmt.Println("No domains attached.")
			return nil
		}
		rows := make([][]string, len(domains))
		for i, d := range domains {
			rows[i] = []string{d}
		}
		output.Table([]string{"DOMAIN"}, rows)
		return nil
	},
}

// ---- Cron history ----

var (
	cronHistPage   int
	cronHistLimit  int
	cronHistStatus string
	cronHistCronID string
	cronHistFrom   string
	cronHistTo     string
)

var cronHistoryCmd = &cobra.Command{
	Use:   "history [machine]",
	Short: "View past cron job runs (status, duration, logs)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		resp, err := client.ListCronHistory(args[0], api.CronHistoryFilter{
			Page:   cronHistPage,
			Limit:  cronHistLimit,
			Status: cronHistStatus,
			CronID: cronHistCronID,
			From:   cronHistFrom,
			To:     cronHistTo,
		})
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(resp)
		}
		if len(resp.Runs) == 0 {
			fmt.Println("No runs.")
			return nil
		}
		rows := make([][]string, len(resp.Runs))
		for i, r := range resp.Runs {
			started := "-"
			if r.StartedAt != nil {
				started = *r.StartedAt
			}
			completed := "-"
			if r.CompletedAt != nil {
				completed = *r.CompletedAt
			}
			rows[i] = []string{r.CronName, r.Status, started, completed}
		}
		output.Table([]string{"CRON", "STATUS", "STARTED", "COMPLETED"}, rows)
		fmt.Printf("\nPage %d/%d (limit %d, total %d)\n",
			resp.Page, (resp.Total+resp.Limit-1)/maxInt(resp.Limit, 1), resp.Limit, resp.Total)
		return nil
	},
}

// ---- Active PRs delete ----

var prsDeleteCmd = &cobra.Command{
	Use:   "delete [machine] <pr-number>",
	Short: "Tear down a PR preview environment",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		num, err := strconv.Atoi(strings.TrimPrefix(args[1], "#"))
		if err != nil {
			return fmt.Errorf("invalid PR number: %w", err)
		}
		if err := client.DeleteActivePR(args[0], num); err != nil {
			return err
		}
		fmt.Printf("✓ PR #%d preview environment deleted\n", num)
		return nil
	},
}

// ---- Deployment cancel ----

var cancelDeploymentCmd = &cobra.Command{
	Use:   "cancel [machine] <deployment-id>",
	Short: "Cancel a running deployment (kills the build job)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if err := client.CancelDeployment(args[0], args[1]); err != nil {
			return err
		}
		fmt.Println("✓ Deployment cancelled")
		return nil
	},
}

// ---- Trial status ----

var trialStatusCmd = &cobra.Command{
	Use:   "trial-status",
	Short: "Show current user trial status (days left)",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		s, err := client.GetTrialStatus()
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(s)
		}
		if !s.IsOnTrial {
			fmt.Println("Not on trial.")
			return nil
		}
		fmt.Printf("Trial ends: %s (%d days left)\n", s.TrialEndsAt, s.DaysLeft)
		return nil
	},
}

// ---- Storage usage ----

var storageUsageCmd = &cobra.Command{
	Use:   "storage [machine]",
	Short: "Show S3 storage usage for a machine",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		resp, err := client.GetStorageUsage(args[0])
		if err != nil {
			return err
		}
		return output.JSON(resp)
	},
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func init() {
	stackDetectCmd.Flags().Int64Var(&stackInstallID, "installation-id", 0, "GitHub App installation ID (required)")
	stackDetectCmd.Flags().StringVar(&stackOwner, "owner", "", "Repo owner (required)")
	stackDetectCmd.Flags().StringVar(&stackRepo, "repo", "", "Repo name (required)")
	stackDetectCmd.Flags().StringVar(&stackRef, "ref", "main", "Git ref (branch, tag, or SHA)")
	stackDetectCmd.MarkFlagRequired("installation-id")
	stackDetectCmd.MarkFlagRequired("owner")
	stackDetectCmd.MarkFlagRequired("repo")

	billingProjectPortalCmd.Flags().StringVar(&billingProjPortalReturn, "return-url", "https://manager.usectl.com", "Stripe portal return URL")

	priceCmd.Flags().Float64Var(&priceVCPU, "vcpu", 0, "vCPU count (required)")
	priceCmd.Flags().Float64Var(&priceRAMGB, "ram-gb", 0, "RAM in GiB (required)")
	priceCmd.Flags().Float64Var(&priceStorageGB, "storage-gb", 0, "Storage in GiB")
	priceCmd.Flags().StringVar(&priceInterval, "interval", "month", "Billing interval: month | year")
	priceCmd.MarkFlagRequired("vcpu")
	priceCmd.MarkFlagRequired("ram-gb")

	cronHistoryCmd.Flags().IntVar(&cronHistPage, "page", 1, "Page number")
	cronHistoryCmd.Flags().IntVar(&cronHistLimit, "limit", 10, "Items per page (max 100)")
	cronHistoryCmd.Flags().StringVar(&cronHistStatus, "status", "", "Filter: succeeded | failed | running")
	cronHistoryCmd.Flags().StringVar(&cronHistCronID, "cron", "", "Filter by cron name")
	cronHistoryCmd.Flags().StringVar(&cronHistFrom, "from", "", "RFC3339 start time")
	cronHistoryCmd.Flags().StringVar(&cronHistTo, "to", "", "RFC3339 end time")

	billingProjectCmd.AddCommand(billingProjectGetCmd, billingProjectPortalCmd)

	rootCmd.AddCommand(stackDetectCmd, billingProjectCmd, priceCmd, projectDomainsListCmd,
		trialStatusCmd, storageUsageCmd)

	// Wire `cron history` and `pr delete` and `deployments cancel` into the existing groups.
	cronCmd.AddCommand(cronHistoryCmd)
	projectsPRsCmd.AddCommand(prsDeleteCmd)
	projectsDeploymentsCmd.AddCommand(cancelDeploymentCmd)
}
