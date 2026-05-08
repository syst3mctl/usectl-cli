package cmd

import (
	"fmt"
	"strconv"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

var quotaCmd = &cobra.Command{
	Use:   "quota",
	Short: "Inspect a machine's resource quota and pressure status",
	Long: `Shows the machine's CPU/RAM/storage budget vs live usage from the namespace
ResourceQuota. Includes per-app autoscale envelopes (min..max replicas) and
recommendations when the machine is approaching or hitting its quota
ceiling (upgrade plan, scale down, or rollover legacy oversized pods).`,
}

var quotaGetCmd = &cobra.Command{
	Use:   "get <project-id>",
	Short: "Get quota status for a machine",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		q, err := client.GetProjectQuota(args[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(q)
		}
		fmt.Printf("Status: %s\n", q.Status)
		applied := "no"
		if q.Applied {
			applied = "yes"
		}
		output.Table([]string{"FIELD", "TOTAL", "USED"}, [][]string{
			{"vCPU", fmt.Sprintf("%.2f", q.VCPUTotal), fmt.Sprintf("%dm", q.VCPUUsedMillis)},
			{"RAM (GiB)", fmt.Sprintf("%.2f", q.RAMGBTotal), fmt.Sprintf("%d MiB", q.RAMUsedMiB)},
			{"Storage (GiB)", fmt.Sprintf("%.2f", q.StorageGBTotal), fmt.Sprintf("%d GiB", q.StorageUsedGiB)},
		})
		fmt.Printf("\nNamespace ResourceQuota applied: %s\n", applied)
		fmt.Printf("Per-pod default CPU: %dm  RAM: %d MiB\n", q.PerPodCPUMillis, q.PerPodRAMMiB)
		fmt.Printf("Recent admission failures: %d\n", q.AdmissionFailuresRecent)

		if len(q.Apps) > 0 {
			fmt.Println()
			rows := make([][]string, len(q.Apps))
			for i, a := range q.Apps {
				rows[i] = []string{a.ID, a.Name,
					strconv.Itoa(a.MinReplicas),
					strconv.Itoa(int(a.MaxReplicas)),
					strconv.Itoa(int(a.CurrentReplicas)),
				}
			}
			output.Table([]string{"APP ID", "NAME", "MIN", "MAX", "CURRENT"}, rows)
		}
		if len(q.LegacyOversizedPods) > 0 {
			fmt.Println("\nLegacy oversized pods (consider `usectl quota rollover`):")
			rows := make([][]string, len(q.LegacyOversizedPods))
			for i, p := range q.LegacyOversizedPods {
				rows[i] = []string{p.Name,
					fmt.Sprintf("%dm", p.DeclaredCPUMillis),
					fmt.Sprintf("%d MiB", p.DeclaredMemoryMiB)}
			}
			output.Table([]string{"POD", "CPU", "MEM"}, rows)
		}
		if q.Recommendation != nil && q.Recommendation.Action != "" {
			fmt.Printf("\n⚠ Recommendation: %s — %s\n", q.Recommendation.Action, q.Recommendation.Message)
			if q.Recommendation.SuggestedPlan != nil {
				p := q.Recommendation.SuggestedPlan
				fmt.Printf("  Suggested plan: %.2f vCPU / %.2f GiB RAM / %.2f GiB storage (%d cents/mo)\n",
					p.VCPU, p.RAMGB, p.StorageGB, p.MonthlyPriceCents)
			}
		}
		return nil
	},
}

var (
	quotaPreviewVCPU      float64
	quotaPreviewRAMGB     float64
	quotaPreviewStorageGB float64
)

var quotaPreviewCmd = &cobra.Command{
	Use:   "preview <project-id>",
	Short: "Dry-run a plan resize and check if the new totals fit the existing apps",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		req := api.QuotaPreviewRequest{
			VCPU:      quotaPreviewVCPU,
			RAMGB:     quotaPreviewRAMGB,
			StorageGB: quotaPreviewStorageGB,
		}
		resp, err := client.PreviewQuotaChange(args[0], req)
		if err != nil {
			return err
		}
		return output.JSON(resp)
	},
}

var quotaRolloverCmd = &cobra.Command{
	Use:   "rollover <project-id>",
	Short: "Restart legacy oversized pods so they come back under the per-pod default",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if err := client.RolloverLegacyPods(args[0]); err != nil {
			return err
		}
		fmt.Println("✓ Rollover triggered. Pods will restart shortly.")
		return nil
	},
}

func init() {
	quotaPreviewCmd.Flags().Float64Var(&quotaPreviewVCPU, "vcpu", 0, "Proposed vCPU total (required)")
	quotaPreviewCmd.Flags().Float64Var(&quotaPreviewRAMGB, "ram-gb", 0, "Proposed RAM in GiB (required)")
	quotaPreviewCmd.Flags().Float64Var(&quotaPreviewStorageGB, "storage-gb", 0, "Proposed storage in GiB")
	quotaPreviewCmd.MarkFlagRequired("vcpu")
	quotaPreviewCmd.MarkFlagRequired("ram-gb")

	quotaCmd.AddCommand(quotaGetCmd, quotaPreviewCmd, quotaRolloverCmd)
	rootCmd.AddCommand(quotaCmd)
}
