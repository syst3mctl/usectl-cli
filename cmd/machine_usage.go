package cmd

import (
	"fmt"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

// `usectl machines usage` — what the machine is consuming against what it pays
// for.
//
// The numbers come from the quota endpoint, which reports allocation (the sum
// of what pods have RESERVED via requests/limits), not live utilisation. That
// distinction is the whole point of the view: a machine can be at 100% of its
// CPU quota while every pod sits idle, and the fix is resizing pods, not
// buying a bigger plan. The header says so rather than leaving the reader to
// infer it.

var machineUsageCmd = &cobra.Command{
	Use:     "usage [machine]",
	Aliases: []string{"limits"},
	Short:   "Show resource usage against the machine's limits",
	Long: `Show how much of the machine's vCPU, RAM and storage its pods have reserved,
and how much headroom is left.

These are ALLOCATIONS (the sum of each pod's requests/limits), not live
utilisation — a machine can be full while every pod is idle. Use
'machines pods stats' for live CPU and memory.`,
	Example: `  usectl machines usage api
  usectl machines usage api --json`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		ref, src, err := machineRef(firstOrEmpty(args))
		if err != nil {
			return err
		}
		echoMachineSource(ref, src)
		machineID, err := resolveMachine(client, ref)
		if err != nil {
			return err
		}
		q, err := client.GetProjectQuota(machineID)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(q)
		}

		usedCPU := float64(q.VCPUUsedMillis) / 1000
		usedRAM := float64(q.RAMUsedMiB) / 1024
		usedSto := float64(q.StorageUsedGiB)

		fmt.Printf("Allocated vs limit  (reservations, not live utilisation)\n\n")
		output.Table([]string{"RESOURCE", "ALLOCATED", "LIMIT", "FREE", "USED"}, [][]string{
			usageRow("vCPU", usedCPU, q.VCPUTotal, ""),
			usageRow("RAM", usedRAM, q.RAMGBTotal, "GB"),
			usageRow("Storage", usedSto, q.StorageGBTotal, "GB"),
		})

		if q.Status != "" {
			fmt.Printf("\n  status            %s\n", q.Status)
		}
		if !q.Applied {
			// A quota that exists in the database but was never applied to the
			// namespace enforces nothing, so the numbers above are advisory.
			fmt.Println(output.Yellow("  ⚠ quota not applied to the namespace — these limits are not being enforced"))
		}
		if q.AdmissionFailuresRecent > 0 {
			// Pods rejected at admission are the concrete symptom of a full
			// machine, and they do not show up as a failed deploy anywhere else.
			fmt.Println(output.Red(fmt.Sprintf("  ⚠ %d pod(s) recently rejected at admission — the machine is out of room",
				q.AdmissionFailuresRecent)))
		}
		if n := len(q.LegacyOversizedPods); n > 0 {
			fmt.Printf("  ⚠ %d legacy pod(s) exceed the per-pod default; 'usectl machines quota rollover %s' restarts them\n",
				n, args[0])
		}
		if q.PerPodCPUMillis > 0 || q.PerPodRAMMiB > 0 {
			fmt.Printf("\n  per-pod default   %dm cpu · %dMi ram\n", q.PerPodCPUMillis, q.PerPodRAMMiB)
		}

		// The quota endpoint reports only replica counts per app, so the
		// per-pod sizing is joined in from the app records — that is where
		// cpu_millis / memory_mib / storage_mib live.
		if len(q.Apps) > 0 {
			sizing := map[string]api.ProjectApp{}
			if apps, aErr := client.ListProjectApps(machineID); aErr == nil {
				for _, a := range apps {
					sizing[a.ID] = a
				}
			}
			fmt.Println("\nPer pod")
			rows := make([][]string, len(q.Apps))
			for i, a := range q.Apps {
				app := sizing[a.ID]
				rows[i] = []string{
					a.Name,
					intPtrOr(app.CPUMillis, "250 (def)", "m"),
					intPtrOr(app.MemoryMiB, "256 (def)", "Mi"),
					intPtrOr(app.StorageMiB, "2048 (def)", "Mi"),
					fmt.Sprintf("%d/%d", a.CurrentReplicas, a.MaxReplicas),
				}
			}
			output.Table([]string{"POD", "CPU", "RAM", "STORAGE", "REPLICAS"}, rows)

			// The rows above cover app pods only. Addon pods (a dedicated
			// Postgres pair can dwarf the apps), cron jobs and DB UIs draw on
			// the same wallet, so the remainder is stated explicitly rather
			// than leaving the reader to wonder why the column does not add up.
			var appCPU, appRAM float64
			for _, a := range q.Apps {
				app := sizing[a.ID]
				reps := float64(a.CurrentReplicas)
				if reps == 0 {
					reps = 1
				}
				appCPU += float64(intOr(app.CPUMillis, 250)) / 1000 * reps
				appRAM += float64(intOr(app.MemoryMiB, 256)) / 1024 * reps
			}
			if restCPU, restRAM := usedCPU-appCPU, usedRAM-appRAM; restCPU > 0.01 || restRAM > 0.01 {
				fmt.Printf("\n  addons, cron jobs and DB UIs account for the rest: %gm cpu · %gGB ram\n",
					round2(restCPU*1000), round2(restRAM))
			}
		}

		if q.Recommendation != nil && q.Recommendation.Message != "" {
			fmt.Printf("\n  recommendation    %s\n", q.Recommendation.Message)
			if p := q.Recommendation.SuggestedPlan; p != nil {
				fmt.Printf("  suggested plan    %g vCPU · %g GB RAM · %g GB storage",
					p.VCPU, p.RAMGB, p.StorageGB)
				if p.MonthlyPriceCents > 0 {
					fmt.Printf("  ($%.2f/mo)", float64(p.MonthlyPriceCents)/100)
				}
				fmt.Println()
			}
		}
		return nil
	},
}

func intOr(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

func usageRow(label string, used, total float64, unit string) []string {
	free := total - used
	if free < 0 {
		free = 0
	}
	pct := "—"
	if total > 0 {
		frac := used / total
		txt := fmt.Sprintf("%3.0f%%  %s", frac*100, usageBar(frac))
		switch {
		case frac >= 0.9:
			pct = output.Red(txt)
		case frac >= 0.7:
			pct = output.Yellow(txt)
		default:
			pct = output.Green(txt)
		}
	}
	return []string{
		label,
		fmt.Sprintf("%g%s", round2(used), unit),
		fmt.Sprintf("%g%s", round2(total), unit),
		fmt.Sprintf("%g%s", round2(free), unit),
		pct,
	}
}

func usageBar(frac float64) string {
	const width = 20
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	filled := int(frac*width + 0.5)
	return "[" + strings.Repeat("#", filled) + strings.Repeat(".", width-filled) + "]"
}

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}
