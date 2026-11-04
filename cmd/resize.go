package cmd

import (
	"fmt"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/config"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

var (
	resizeVCPU     float64
	resizeRAM      string
	resizeStorage  string
	resizeInterval string
	resizeNoOpen   bool
)

var projectsResizeCmd = &cobra.Command{
	Use:   "resize [machine] [--vcpu N] [--ram SIZE] [--storage SIZE]",
	Short: "Change a machine's vCPU, RAM or storage",
	Long: `Resize a machine's resource wallet. Flags you leave out keep their current
value. The new price is shown and confirmed before anything changes.

What happens next depends on the machine:

  · already paying     the subscription's price is updated in place and the
                       difference is prorated on the next invoice — no payment
                       page, the new size applies immediately
  · in its trial       the size applies immediately while it stays within the
                       trial limits
  · otherwise          a Stripe checkout link opens; the size applies once the
                       payment completes

The server refuses a size too small for the machine's pods and addons.`,
	Example: `  usectl machines resize api --ram 8
  usectl machines resize api --vcpu 2 --ram 4gb --storage 40
  usectl machines resize api --ram 8 --yes`,
	Args: cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}
		machineID := args[0]

		cur, err := client.GetProjectBilling(machineID)
		if err != nil {
			return fmt.Errorf("read current size: %w", err)
		}
		vcpu, ram, storage := cur.VCPU, cur.RAMGB, cur.StorageGB
		changed := false
		if cmd.Flags().Changed("vcpu") {
			if resizeVCPU <= 0 {
				return fmt.Errorf("--vcpu must be greater than zero")
			}
			vcpu, changed = resizeVCPU, true
		}
		if cmd.Flags().Changed("ram") {
			if ram, err = parseSizeGB(resizeRAM); err != nil {
				return fmt.Errorf("--ram: %w", err)
			}
			changed = true
		}
		if cmd.Flags().Changed("storage") {
			if storage, err = parseSizeGB(resizeStorage); err != nil {
				return fmt.Errorf("--storage: %w", err)
			}
			changed = true
		}
		if !changed {
			return fmt.Errorf("nothing to change — pass at least one of --vcpu, --ram, --storage")
		}
		interval := resizeInterval
		if interval == "" {
			interval = cur.BillingInterval
		}
		if interval != "month" && interval != "year" {
			interval = "month"
		}

		// The server validates amount_cents against its own calculation, so
		// ask it for the price rather than computing one here.
		price, err := client.CalculatePrice(vcpu, ram, storage, interval)
		if err != nil {
			return fmt.Errorf("price the new size: %w", err)
		}

		fmt.Printf("  %-9s %s vCPU · %s GB RAM · %s GB storage\n", "Current",
			trimFloat(cur.VCPU), trimFloat(cur.RAMGB), trimFloat(cur.StorageGB))
		fmt.Printf("  %-9s %s vCPU · %s GB RAM · %s GB storage\n", "New",
			trimFloat(vcpu), trimFloat(ram), trimFloat(storage))
		fmt.Printf("  %-9s $%.2f / %s\n", "Price", float64(price.IntervalAmount)/100, interval)
		if !assumeYes {
			if !interactive() {
				return fmt.Errorf("refusing to resize without confirmation — pass --yes to proceed non-interactively")
			}
			if !confirm("Apply this size?") {
				return fmt.Errorf("cancelled")
			}
		}

		// Stripe needs somewhere to send the browser back to; the machine's
		// billing tab is the natural landing page. Same base-URL rule as
		// `billing subscribe`: the configured API host serves the dashboard.
		base := "https://manager.usectl.com"
		if cfg, _ := config.Load(); cfg != nil && cfg.APIURL != "" {
			base = strings.TrimSuffix(strings.TrimSuffix(cfg.APIURL, "/"), "/api")
		}
		back := fmt.Sprintf("%s/machines/%s/billing", base, machineID)
		res, err := client.CreateProjectCheckout(machineID, api.ProjectCheckoutRequest{
			AmountCents: price.IntervalAmount,
			Interval:    interval,
			VCPU:        vcpu,
			RAMGB:       ram,
			StorageGB:   storage,
			SuccessURL:  back,
			CancelURL:   back,
		})
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(res)
		}
		if !res.PaymentRequired || res.URL == "" {
			msg := res.Message
			if msg == "" {
				msg = "Resources updated."
			}
			fmt.Printf("%s %s\n", output.Green("✓"), msg)
			return nil
		}
		fmt.Printf("\nComplete the payment to apply the new size:\n  %s\n", res.URL)
		if !resizeNoOpen && interactive() {
			_ = openBrowser(res.URL)
		}
		return nil
	},
}

func init() {
	f := projectsResizeCmd.Flags()
	f.Float64Var(&resizeVCPU, "vcpu", 0, "vCPU count, e.g. 2 or 0.5")
	f.StringVar(&resizeRAM, "ram", "", "RAM in GB, or with a suffix: 4gb, 512mb")
	f.StringVar(&resizeStorage, "storage", "", "Storage in GB, or with a suffix: 40gb")
	f.StringVar(&resizeInterval, "interval", "", "Billing interval: month or year (default: the current one)")
	f.BoolVar(&resizeNoOpen, "no-open", false, "Print the checkout link instead of opening a browser")
	f.BoolVarP(&assumeYes, "yes", "y", false, "Apply without the confirmation prompt")
	projectsCmd.AddCommand(projectsResizeCmd)
}
