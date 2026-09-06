package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// `usectl machines enter <machine>` — an interactive sub-shell scoped to one
// machine, so a session of related work does not repeat the machine on every
// line.
//
// Commands typed inside are resolved against `machines` first and then against
// the root, so both `pods` and `machines pods` work, as does `billing`.
//
// This is a HUMAN affordance. It is not scriptable and agents should use the
// explicit argument or -m instead; see the AI guide in the root help.

var enterCmd = &cobra.Command{
	Use:   "enter [machine]",
	Short: "Open an interactive shell scoped to one machine",
	Long: `Start a sub-shell in which every command applies to one machine, so the
machine does not have to be named each time.

  usectl(api)> pods
  usectl(api)> pods set web port=8080
  usectl(api)> addons list
  usectl(api)> exit

Commands are looked up under 'machines' first, then at the top level. Type
'help' for the command list, 'exit' or Ctrl-D to leave.

Line editing is basic: there is no history or arrow-key support.`,
	Example: `  usectl machines enter api
  usectl machines enter          # uses the machine from 'usectl use'`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, src, err := machineRef(firstOrEmpty(args))
		if err != nil {
			return err
		}
		echoMachineSource(ref, src)

		client, cErr := api.NewClient(apiURL)
		if cErr != nil {
			return cErr
		}
		// Resolve up front: entering a shell bound to a machine that does not
		// exist would fail identically on every subsequent line.
		if _, err := resolveMachine(client, ref); err != nil {
			return err
		}

		fmt.Printf("Scoped to machine %q. 'help' for commands, 'exit' to leave.\n", ref)
		inMachineShell = true
		defer func() { inMachineShell = false }()
		reader := bufio.NewReader(os.Stdin)
		for {
			fmt.Printf("usectl(%s)> ", ref)
			line, rErr := reader.ReadString('\n')
			if rErr == io.EOF {
				fmt.Println()
				return nil
			}
			if rErr != nil {
				return rErr
			}
			line = strings.TrimSpace(line)
			switch line {
			case "":
				continue
			case "exit", "quit", "\\q":
				return nil
			}

			fields, sErr := splitArgs(line)
			if sErr != nil {
				fmt.Fprintf(os.Stderr, "  %v\n", sErr)
				continue
			}
			if err := runInShell(ref, fields); err != nil {
				// A failed command must not end the session — that is the
				// difference between a shell and a one-shot invocation.
				fmt.Fprintf(os.Stderr, "  %v\n", err)
			}
		}
	},
}

// noMachineArg lists machine-scoped commands that do NOT take a machine as
// their first positional, so the shell must not inject one.
var noMachineArg = map[string]bool{
	"usectl machines list":   true,
	"usectl machines create": true,
	"usectl machines enter":  true,
}

// runInShell dispatches one typed line with the machine supplied for the user.
//
// The machine is injected as the first POSITIONAL argument rather than left to
// -m, because almost every machine-scoped command declares it positionally and
// cobra rejects the call for arity before any flag could stand in for it.
// cobra's own Find() is used to locate the boundary between the subcommand
// path and its arguments, so the injection point is correct regardless of how
// deeply nested the command is (pods set, addons get, ...).
func runInShell(machine string, fields []string) error {
	resetFlags(rootCmd)
	machineFlag = machine

	// Prefer the machine-scoped spelling: inside the shell "pods" means
	// "machines pods". Fall back to the root so top-level commands
	// (billing, github, orgs) still work.
	candidate := append([]string{"machines"}, fields...)
	target, rest, err := rootCmd.Find(candidate)
	// Find() does not fail on an unknown subcommand: it returns the deepest
	// command it DID match plus the leftovers. So "trial-status" resolves to
	// `machines` itself with rest=["trial-status"], which would print the
	// machines help instead of running the top-level command. Detect that by
	// checking whether the first typed word was actually consumed.
	unresolved := err != nil || target == rootCmd ||
		(len(fields) > 0 && len(rest) > 0 && rest[0] == fields[0])
	if unresolved {
		target, rest, err = rootCmd.Find(fields)
		if err != nil {
			return err
		}
	}

	path := strings.Fields(target.CommandPath())
	underMachines := len(path) > 1 && path[1] == "machines"
	if underMachines && !noMachineArg[target.CommandPath()] {
		// Only inject when the user did not already name a machine. Comparing
		// against the bound machine covers the common case of typing it
		// anyway out of habit.
		if len(rest) == 0 || rest[0] != machine {
			rest = append([]string{machine}, rest...)
		}
	}

	full := append(path[1:], rest...)
	rootCmd.SetArgs(full)
	defer rootCmd.SetArgs(nil)
	return rootCmd.Execute()
}

// resetFlags restores every flag in the tree to its default between lines.
//
// Cobra keeps flag values on the command objects, so without this a --json on
// one line would silently stay in force for the rest of the session, and a
// --reveal typed once would keep printing secrets.
func resetFlags(c *cobra.Command) {
	reset := func(f *pflag.Flag) {
		if f.Changed {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		}
	}
	c.Flags().VisitAll(reset)
	c.PersistentFlags().VisitAll(reset)
	for _, sub := range c.Commands() {
		resetFlags(sub)
	}
}

// splitArgs is a small shell-style splitter honouring single and double
// quotes, so values containing spaces survive:
//
//	pods set web command="node worker.js"
func splitArgs(line string) ([]string, error) {
	var out []string
	var cur strings.Builder
	var quote rune
	escaped := false
	started := false

	for _, r := range line {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
			started = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			started = true
		case r == ' ' || r == '\t':
			if started {
				out = append(out, cur.String())
				cur.Reset()
				started = false
			}
		default:
			cur.WriteRune(r)
			started = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unclosed %c quote", quote)
	}
	if started {
		out = append(out, cur.String())
	}
	return out, nil
}

func init() {
	projectsCmd.AddCommand(enterCmd)
}
