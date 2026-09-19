package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

// usectl ask — Devo from the terminal. Read-only by construction; when Devo
// proposes a change you get a [y/N] prompt and, on yes, the tool runs as
// you (the same approval path as the dashboard card).

var (
	askMachine   string
	askYes       bool
	askNoActions bool
)

var askCmd = &cobra.Command{
	Use:   "ask <question>",
	Short: "Ask Devo, the platform assistant, about a machine",
	Long: `Devo reads the machine's pods, logs, deployments, diagnostics and probes and
answers in plain language. It cannot change anything itself: when a change is
the fix, it proposes it and you approve it here.

  usectl ask "why is my last deploy failing?" -m my-api
  usectl ask "restart the web pod" -m my-api        # → proposal → [y/N]
  usectl ask "how do I add a custom domain?"        # no machine: product question`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		question := strings.Join(args, " ")
		projectID := ""
		if m := firstNonEmptyStr(askMachine, machineFlag, os.Getenv("USECTL_MACHINE")); m != "" {
			id, err := resolveMachine(client, m)
			if err != nil {
				return err
			}
			projectID = id
		}
		conv := uuid.NewString()
		var pending []api.CopilotAction
		spinnerShown := false
		err = client.Ask(projectID, question, nil, conv, func(ev api.CopilotEvent) {
			switch {
			case ev.Tool != "":
				if !spinnerShown {
					fmt.Fprint(os.Stderr, "… ")
					spinnerShown = true
				}
				fmt.Fprintf(os.Stderr, "[%s] ", ev.Tool)
			case ev.Proposal != nil:
				pending = append(pending, *ev.Proposal)
			case ev.Content != "":
				if spinnerShown {
					fmt.Fprintln(os.Stderr)
					spinnerShown = false
				}
				fmt.Print(ev.Content)
			case ev.Error != "":
				fmt.Fprintf(os.Stderr, "\n(%s)\n", ev.Error)
			}
		})
		fmt.Println()
		if err != nil {
			return err
		}
		if len(pending) == 0 || askNoActions {
			return nil
		}
		for _, p := range pending {
			fmt.Printf("\n▸ Proposed: %s  [%s · %s risk]\n", p.Summary, p.Tool, p.Risk)
			if p.Why != "" {
				fmt.Printf("  %s\n", p.Why)
			}
			if len(p.Args) > 0 {
				fmt.Printf("  args: %v\n", p.Args)
			}
			approve := askYes
			if !approve {
				if !interactive() {
					fmt.Println("  Not approved (non-interactive; pass --yes to approve automatically).")
					continue
				}
				fmt.Print("  Approve and run? [y/N] ")
				line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
				approve = strings.EqualFold(strings.TrimSpace(line), "y")
			}
			if !approve {
				_, _ = client.RejectCopilotAction(projectID, p.ID)
				fmt.Println("  Rejected.")
				continue
			}
			res, err := client.ApproveCopilotAction(projectID, p.ID)
			if err != nil {
				fmt.Printf("  ✗ %v\n", err)
				continue
			}
			out := ""
			if res.Result != nil {
				out = " — " + strings.TrimSpace(*res.Result)
			}
			fmt.Printf("  ✓ %s%s\n", res.Status, out)
		}
		return nil
	},
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func init() {
	askCmd.Flags().StringVar(&askMachine, "machine-name", "", "Machine to scope Devo to (or -m / USECTL_MACHINE)")
	askCmd.Flags().BoolVarP(&askYes, "yes", "y", false, "Approve every proposal without asking (scripts)")
	askCmd.Flags().BoolVar(&askNoActions, "no-actions", false, "Never approve or reject; just print the answer")
	rootCmd.AddCommand(askCmd)
}
