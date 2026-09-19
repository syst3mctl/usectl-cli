package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

// usectl AI: token usage and the per-machine budget (mig 083).
//
// A budget is a calendar-month token cap. Over it, the gateway either refuses
// calls (429 budget_exceeded) or routes them to the platform's local Qwen so
// the app keeps working without charging the tenant's own provider key.

var (
	aiBudgetCap      string
	aiBudgetOnExceed string
	aiBudgetWarn     int
	aiBudgetAlerts   string
	aiBudgetPrice    string
	aiBudgetClear    bool
)

var aiCmd = &cobra.Command{
	Use:   "ai",
	Short: "usectl AI addon: token usage and monthly budget",
}

var aiUsageCmd = &cobra.Command{
	Use:   "usage [machine]",
	Short: "Token usage for the last 30 days, per day and model",
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}
		u, err := client.GetAIUsage(args[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(u)
		}
		if len(u.Usage) == 0 {
			fmt.Println("No AI usage in the last 30 days.")
			return nil
		}
		rows := make([][]string, 0, len(u.Usage))
		for _, d := range u.Usage {
			rows = append(rows, []string{d.Period, d.Model, humanTokens(d.PromptTokens), humanTokens(d.CompletionTokens), humanTokens(d.TotalTokens), strconv.FormatInt(d.Requests, 10)})
		}
		output.Table([]string{"DAY", "MODEL", "PROMPT", "COMPLETION", "TOTAL", "REQUESTS"}, rows)
		fmt.Printf("\n30-day total: %s tokens\n", humanTokens(u.TotalTokens))
		return nil
	},
}

var aiBudgetCmd = &cobra.Command{
	Use:   "budget [machine]",
	Short: "Show or set the machine's monthly AI token budget",
	Long: `Without flags, shows the budget and month-to-date usage.

  usectl ai budget api --cap 5M                    cap at 5 million tokens/month, refuse over it
  usectl ai budget api --cap 5M --on-exceed fallback   ...or fall back to the local Qwen instead
  usectl ai budget api --warn 90 --alerts on       spend alert at 90 %% (owner inbox, email, push)
  usectl ai budget api --price 300                 show a $ estimate at $3.00 per 1M tokens
  usectl ai budget api --clear                     remove the budget

Caps accept k/M/B suffixes. Alerts also fire at 100 %%; each fires once per month.`,
	Args: cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}
		machine := args[0]
		if aiBudgetClear {
			if err := client.DeleteAIBudget(machine); err != nil {
				return err
			}
			fmt.Println("✓ AI budget removed — no cap, no spend alerts.")
			return nil
		}
		changed := cmd.Flags().Changed("cap") || cmd.Flags().Changed("on-exceed") || cmd.Flags().Changed("warn") || cmd.Flags().Changed("alerts") || cmd.Flags().Changed("price")
		var st *api.AIBudgetStatus
		if changed {
			cur, err := client.GetAIBudget(machine)
			if err != nil {
				return err
			}
			b := api.AIBudget{OnExceed: "block", WarnPct: 80, AlertsEnabled: true}
			if cur.Budget != nil {
				b = *cur.Budget
			}
			if cmd.Flags().Changed("cap") {
				if strings.EqualFold(aiBudgetCap, "none") || aiBudgetCap == "0" {
					b.MonthlyTokens = nil
				} else {
					n, err := parseTokens(aiBudgetCap)
					if err != nil {
						return err
					}
					b.MonthlyTokens = &n
				}
			}
			if cmd.Flags().Changed("on-exceed") {
				if aiBudgetOnExceed != "block" && aiBudgetOnExceed != "fallback" {
					return fmt.Errorf("--on-exceed must be block or fallback")
				}
				b.OnExceed = aiBudgetOnExceed
			}
			if cmd.Flags().Changed("warn") {
				b.WarnPct = aiBudgetWarn
			}
			if cmd.Flags().Changed("alerts") {
				switch strings.ToLower(aiBudgetAlerts) {
				case "on", "true", "yes":
					b.AlertsEnabled = true
				case "off", "false", "no":
					b.AlertsEnabled = false
				default:
					return fmt.Errorf("--alerts must be on or off")
				}
			}
			if cmd.Flags().Changed("price") {
				if strings.EqualFold(aiBudgetPrice, "none") {
					b.PricePerMillionCents = nil
				} else {
					c, err := strconv.Atoi(aiBudgetPrice)
					if err != nil || c < 0 {
						return fmt.Errorf("--price is cents per 1M tokens (e.g. 300 = $3.00)")
					}
					b.PricePerMillionCents = &c
				}
			}
			st, err = client.PutAIBudget(machine, b)
			if err != nil {
				return err
			}
		} else {
			st, err = client.GetAIBudget(machine)
			if err != nil {
				return err
			}
		}
		if jsonOutput {
			return output.JSON(st)
		}
		printBudget(st)
		return nil
	},
}

func printBudget(st *api.AIBudgetStatus) {
	if st.Budget == nil || st.Budget.MonthlyTokens == nil {
		fmt.Printf("Budget       none (unlimited)\n")
	} else {
		fmt.Printf("Budget       %s tokens / month\n", humanTokens(*st.Budget.MonthlyTokens))
		fmt.Printf("Used         %s (%.0f%%)\n", humanTokens(st.UsedTokens), st.Pct)
		fmt.Printf("Projected    %s by month end\n", humanTokens(st.ProjectedTokens))
		what := "refuse calls (429 budget_exceeded)"
		if st.Budget.OnExceed == "fallback" {
			what = "fall back to the local Qwen"
		}
		fmt.Printf("At the cap   %s\n", what)
	}
	if st.Budget == nil || st.Budget.MonthlyTokens == nil {
		fmt.Printf("Used         %s this month\n", humanTokens(st.UsedTokens))
	}
	if st.Budget != nil {
		alerts := "off"
		if st.Budget.AlertsEnabled {
			alerts = fmt.Sprintf("at %d%% and 100%%", st.Budget.WarnPct)
		}
		fmt.Printf("Alerts       %s\n", alerts)
		if st.Budget.PricePerMillionCents != nil && st.EstimatedCents != nil {
			fmt.Printf("Estimated    $%.2f so far (at $%.2f per 1M tokens)\n", float64(*st.EstimatedCents)/100, float64(*st.Budget.PricePerMillionCents)/100)
		}
	}
	if st.PlatformCapTokens > 0 {
		fmt.Printf("Platform cap %s tokens / month\n", humanTokens(st.PlatformCapTokens))
	}
	fmt.Printf("Resets       %s\n", st.ResetsAt)
}

// parseTokens accepts 500000, 500k, 5M, 1.5B.
func parseTokens(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "K"):
		mult, s = 1_000, strings.TrimSuffix(s, "K")
	case strings.HasSuffix(s, "M"):
		mult, s = 1_000_000, strings.TrimSuffix(s, "M")
	case strings.HasSuffix(s, "B"):
		mult, s = 1_000_000_000, strings.TrimSuffix(s, "B")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 {
		return 0, fmt.Errorf("--cap must be a token count like 500k, 5M or 1.5B")
	}
	return int64(f * float64(mult)), nil
}

func humanTokens(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/1e9), ".0") + "B"
	case n >= 1_000_000:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/1e6), ".0") + "M"
	case n >= 1_000:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/1e3), ".0") + "k"
	}
	return strconv.FormatInt(n, 10)
}

func init() {
	aiBudgetCmd.Flags().StringVar(&aiBudgetCap, "cap", "", "Monthly token cap (500k, 5M, 1.5B; none = unlimited)")
	aiBudgetCmd.Flags().StringVar(&aiBudgetOnExceed, "on-exceed", "", "What happens over the cap: block | fallback")
	aiBudgetCmd.Flags().IntVar(&aiBudgetWarn, "warn", 80, "Spend alert threshold in percent (1–99)")
	aiBudgetCmd.Flags().StringVar(&aiBudgetAlerts, "alerts", "", "Spend alerts: on | off")
	aiBudgetCmd.Flags().StringVar(&aiBudgetPrice, "price", "", "Cents per 1M tokens for the $ estimate (none to clear)")
	aiBudgetCmd.Flags().BoolVar(&aiBudgetClear, "clear", false, "Remove the budget")
	aiCmd.AddCommand(aiUsageCmd, aiBudgetCmd)
	rootCmd.AddCommand(aiCmd)
}
