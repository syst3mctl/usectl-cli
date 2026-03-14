package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/config"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

var billingCmd = &cobra.Command{
	Use:     "billing",
	Aliases: []string{"bill", "b"},
	Short:   "Manage your subscription and billing",
	Long: `View your current plan, subscription status, and manage billing.

Subcommands:
  status      View your current plan and subscription status
  subscribe   Open Stripe checkout to subscribe to a plan
  portal      Open Stripe billing portal to manage payment methods`,
}

var billingStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "View your current billing status and plan",
	Example: `  usectl billing status
  usectl billing status --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		status, err := client.GetBillingStatus()
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(status)
		}

		// Pretty print
		planDisplay := status.Plan
		switch status.Plan {
		case "pro_founding":
			planDisplay = "Pro — Founding Member ⭐ ($15/mo)"
		case "pro":
			planDisplay = "Pro ($29/mo)"
		case "team":
			planDisplay = "Team ($49/mo)"
		case "free", "":
			planDisplay = "No Plan"
		}

		statusDisplay := status.SubscriptionStatus
		switch status.SubscriptionStatus {
		case "active":
			statusDisplay = "✅ Active"
		case "trialing":
			trialInfo := ""
			if status.TrialEndsAt != nil {
				if t, err := time.Parse(time.RFC3339, *status.TrialEndsAt); err == nil {
					remaining := time.Until(t)
					if remaining > 0 {
						hours := int(remaining.Hours())
						days := hours / 24
						if days > 0 {
							trialInfo = fmt.Sprintf(" (%dd %dh remaining)", days, hours%24)
						} else {
							trialInfo = fmt.Sprintf(" (%dh remaining)", hours)
						}
					} else {
						trialInfo = " (expired)"
					}
				}
			}
			statusDisplay = "🕐 Trial" + trialInfo
		case "past_due":
			statusDisplay = "⚠️  Past Due"
		case "canceled":
			statusDisplay = "❌ Canceled"
		case "", "none":
			statusDisplay = "No Subscription"
		}

		fmt.Println()
		fmt.Printf("  Plan:     %s\n", planDisplay)
		fmt.Printf("  Status:   %s\n", statusDisplay)
		if status.IsFoundingMember {
			fmt.Printf("  Member:   ⭐ Founding Member (50%% off locked forever)\n")
		}
		fmt.Println()

		if status.SubscriptionStatus == "" || status.SubscriptionStatus == "none" || status.SubscriptionStatus == "canceled" {
			fmt.Println("  💡 Run 'usectl billing subscribe' to subscribe to a plan.")
		}
		fmt.Println()
		return nil
	},
}

var billingSubscribeCmd = &cobra.Command{
	Use:   "subscribe [plan]",
	Short: "Open Stripe checkout to subscribe (opens browser)",
	Long: `Opens the Stripe checkout page in your browser to subscribe.

Available plans:
  pro_founding   Pro Founding Member — $15/mo (limited to first 100 members)
  team           Team — $49/mo (includes org features)`,
	Example: `  usectl billing subscribe pro_founding
  usectl billing subscribe team`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		plan := strings.ToLower(args[0])
		if plan != "pro_founding" && plan != "team" {
			return fmt.Errorf("invalid plan %q — choose 'pro_founding' or 'team'", plan)
		}

		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}

		cfg, _ := config.Load()
		baseURL := cfg.APIURL
		if baseURL == "" {
			baseURL = "https://manager.usectl.com"
		}

		url, err := client.CreateCheckoutSession(plan,
			baseURL+"/billing?success=true",
			baseURL+"/billing",
		)
		if err != nil {
			return err
		}

		fmt.Printf("Opening checkout in browser...\n")
		if jsonOutput {
			return output.JSON(map[string]string{"url": url})
		}
		return openBrowser(url)
	},
}

var billingPortalCmd = &cobra.Command{
	Use:   "portal",
	Short: "Open Stripe billing portal to manage payment methods (opens browser)",
	Example: `  usectl billing portal`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}

		cfg, _ := config.Load()
		baseURL := cfg.APIURL
		if baseURL == "" {
			baseURL = "https://manager.usectl.com"
		}

		url, err := client.CreateBillingPortal(baseURL + "/billing")
		if err != nil {
			return err
		}

		fmt.Printf("Opening billing portal in browser...\n")
		if jsonOutput {
			return output.JSON(map[string]string{"url": url})
		}
		return openBrowser(url)
	},
}

func init() {
	billingCmd.AddCommand(billingStatusCmd)
	billingCmd.AddCommand(billingSubscribeCmd)
	billingCmd.AddCommand(billingPortalCmd)
	rootCmd.AddCommand(billingCmd)
}
