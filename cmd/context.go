package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/config"
	"github.com/spf13/cobra"
)

// Machine context — not retyping the machine on every command.
//
// Four ways to name a machine, in strict precedence order:
//
//	1. the explicit positional argument
//	2. -m / --machine
//	3. $USECTL_MACHINE
//	4. the persisted default from `usectl use`
//
// Explicit always wins, so an out-of-date context file can never redirect a
// command that already names its target. Whenever a machine is resolved from
// anything BUT the explicit argument, the source is printed — implicit context
// plus `delete` is how the wrong machine gets destroyed, and the one-line echo
// is what makes that visible before it happens.
//
// For agents: use the explicit argument or -m. `enter` is a human affordance
// and is not scriptable.

var machineFlag string

// machineSource describes where a machine reference came from, for the echo.
type machineSource string

const (
	srcArg     machineSource = "argument"
	srcFlag    machineSource = "--machine"
	srcEnv     machineSource = "USECTL_MACHINE"
	srcContext machineSource = "usectl use"
)

// machineRef returns the machine reference to act on, given whatever the
// command received positionally (possibly empty).
func machineRef(arg string) (string, machineSource, error) {
	if arg != "" {
		return arg, srcArg, nil
	}
	if machineFlag != "" {
		return machineFlag, srcFlag, nil
	}
	if v := os.Getenv("USECTL_MACHINE"); v != "" {
		return v, srcEnv, nil
	}
	if cfg, err := config.Load(); err == nil && cfg.Machine != "" {
		return cfg.Machine, srcContext, nil
	}
	return "", "", fmt.Errorf("no machine given — pass one as an argument, use -m, set USECTL_MACHINE, or run 'usectl use <machine>'")
}

// inMachineShell suppresses the source echo inside `machines enter`, where the
// machine is already in the prompt on every line.
var inMachineShell bool

// echoPodSource mirrors echoMachineSource for an implicitly-chosen pod.
func echoPodSource(ref string, src machineSource) {
	if src == srcArg || src == "" || jsonOutput || inMachineShell {
		return
	}
	fmt.Fprintf(os.Stderr, "→ pod %s (from %s)\n", ref, src)
}

// echoMachineSource prints where an implicitly-chosen machine came from.
func echoMachineSource(ref string, src machineSource) {
	if src == srcArg || jsonOutput || inMachineShell {
		return
	}
	fmt.Fprintf(os.Stderr, "→ machine %s (from %s)\n", ref, src)
}

// podRef resolves a pod reference the same way machineRef resolves a machine:
// what was typed wins, then $USECTL_POD, then the default from `usectl use`.
func podRef(given string) (string, machineSource) {
	if given != "" {
		return given, srcArg
	}
	if v := os.Getenv("USECTL_POD"); v != "" {
		return v, srcEnv
	}
	if cfg, err := config.Load(); err == nil && cfg.Pod != "" {
		return cfg.Pod, srcContext
	}
	return "", ""
}

var useCmd = &cobra.Command{
	Use:   "use [machine] [pod]",
	Short: "Set the default machine (and optionally pod) for later commands",
	Long: `Persist a default machine in ~/.usectl/config.json so machine-scoped
commands can be run without naming it every time.

Precedence, highest first: an explicit argument, then -m/--machine, then
$USECTL_MACHINE, then this setting. Commands that fall back to anything other
than an explicit argument print which source they used.

Run with no argument to show the current default; 'usectl use --clear' removes it.`,
	Example: `  usectl use api
  usectl machines pods            # acts on api
  usectl use --clear`,
	Args: cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if useClear {
			cfg.Machine, cfg.Pod = "", ""
			if err := config.Save(cfg); err != nil {
				return err
			}
			fmt.Println("✓ Defaults cleared.")
			return nil
		}
		if len(args) == 0 {
			if cfg.Machine == "" {
				fmt.Println("No default machine set.")
				return nil
			}
			fmt.Printf("Default machine: %s\n", cfg.Machine)
			if cfg.Pod != "" {
				fmt.Printf("Default pod:     %s\n", cfg.Pod)
			}
			return nil
		}

		// Resolve before saving so a typo is caught now rather than surfacing
		// as a confusing failure on some later, unrelated command.
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if _, err := resolveMachine(client, args[0]); err != nil {
			return err
		}
		// A stored pod name only means something inside one machine, so
		// switching machines must not leave the old pod pointing at nothing.
		if cfg.Machine != args[0] {
			cfg.Pod = ""
		}
		cfg.Machine = args[0]

		if len(args) == 2 {
			mID, mErr := resolveMachine(client, args[0])
			if mErr != nil {
				return mErr
			}
			if _, pErr := resolvePod(client, mID, args[1]); pErr != nil {
				return pErr
			}
			cfg.Pod = args[1]
		}
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Printf("✓ Default machine: %s\n", args[0])
		if cfg.Pod != "" {
			fmt.Printf("✓ Default pod:     %s\n", cfg.Pod)
		}
		return nil
	},
}

var useClear bool

func init() {
	useCmd.Flags().BoolVar(&useClear, "clear", false, "Remove the persisted default machine")
	rootCmd.PersistentFlags().StringVarP(&machineFlag, "machine", "m", "", "Machine to act on (overrides USECTL_MACHINE and 'usectl use')")
	rootCmd.AddCommand(useCmd)
}

// firstOrEmpty returns args[0] or "" so a command can accept an optional
// machine argument and let machineRef apply the context chain.
func firstOrEmpty(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

// resolveMachineAndPod accepts both spellings of a pod-scoped command:
//
//	<machine> <pod> [rest...]   explicit
//	<pod> [rest...]             machine from -m / $USECTL_MACHINE / `usectl use`
//
// Disambiguated by TRYING the explicit form first and only falling back when it
// does not resolve, rather than by counting arguments. Counting breaks on the
// commands that take trailing key=value pairs: in
//
//	pods set dns-check rollout=recreate
//
// there are two leading arguments, but the second is a setting, not a pod —
// which is exactly the case that read "dns-check" as a machine and failed with
// a confusing error.
//
// A pod sharing its name with a different machine would bind the explicit form
// first. That is the documented precedence (explicit beats context) and is why
// the fallback path echoes the machine it chose.
func resolveMachineAndPod(client *api.Client, args []string) (machineID, podID string, rest []string, err error) {
	// key=value pairs may appear anywhere, so lift them out before looking for
	// the machine/pod. Without this, `pods set rollout=recreate web` treated
	// "rollout=recreate" as the machine and failed with a nonsense error.
	// A pod or machine name can never contain "=", so the split is exact.
	var positional, pairs []string
	for _, a := range args {
		if strings.Contains(a, "=") {
			pairs = append(pairs, a)
		} else {
			positional = append(positional, a)
		}
	}
	if len(pairs) > 0 {
		mID, pID, extra, rErr := resolveMachineAndPod(client, positional)
		if rErr != nil {
			return "", "", nil, rErr
		}
		return mID, pID, append(extra, pairs...), nil
	}
	args = positional

	if len(args) >= 2 {
		// Documented order first: <machine> <pod>.
		if mID, mErr := resolveMachine(client, args[0]); mErr == nil {
			if pID, pErr := resolvePod(client, mID, args[1]); pErr == nil {
				return mID, pID, args[2:], nil
			}
		}
		// …then the reverse, <pod> <machine>. Machine and pod names resolve
		// against different collections, so whichever way round they are
		// typed only one reading can succeed. Insisting on the documented
		// order buys nothing and is the mistake people actually make.
		if mID, mErr := resolveMachine(client, args[1]); mErr == nil {
			if pID, pErr := resolvePod(client, mID, args[0]); pErr == nil {
				return mID, pID, args[2:], nil
			}
		}
	}
	if len(args) == 0 {
		// No pod named: fall back to the default from `usectl use <m> <pod>`.
		pRef, pSrc := podRef("")
		if pRef == "" {
			return "", "", nil, fmt.Errorf("a pod is required — name one, or set a default with 'usectl use <machine> <pod>'")
		}
		mRef, mSrc, mErr := machineRef("")
		if mErr != nil {
			return "", "", nil, mErr
		}
		echoMachineSource(mRef, mSrc)
		echoPodSource(pRef, pSrc)
		mID, rErr := resolveMachine(client, mRef)
		if rErr != nil {
			return "", "", nil, rErr
		}
		pID, rErr := resolvePod(client, mID, pRef)
		return mID, pID, nil, rErr
	}
	ref, src, cErr := machineRef("")
	if cErr != nil {
		// No context to fall back on, so report what the explicit form
		// actually failed on rather than a generic "no machine given".
		if len(args) >= 2 {
			// Report against whichever argument IS a machine, so the message
			// names the lookup that actually failed rather than blaming the
			// first positional regardless of order.
			if mID, mErr := resolveMachine(client, args[0]); mErr == nil {
				_, pErr := resolvePod(client, mID, args[1])
				return "", "", nil, pErr
			}
			if mID, mErr := resolveMachine(client, args[1]); mErr == nil {
				_, pErr := resolvePod(client, mID, args[0])
				return "", "", nil, pErr
			}
			return "", "", nil, fmt.Errorf("neither %q nor %q is a machine — run 'usectl machines list'", args[0], args[1])
		}
		// A single argument that IS a machine is the commonest mistake here:
		// the user named the machine and left the pod out. Saying so — and
		// listing the pods — beats "no machine given", which is doubly
		// confusing when they just gave one.
		if mID, mErr := resolveMachine(client, args[0]); mErr == nil {
			names := []string{}
			if apps, aErr := client.ListProjectApps(mID); aErr == nil {
				for _, a := range apps {
					names = append(names, a.Name)
				}
				sort.Strings(names)
			}
			hint := ""
			if len(names) > 0 {
				hint = " — pods here: " + strings.Join(names, ", ")
			}
			return "", "", nil, fmt.Errorf("%q is a machine, not a pod; name the pod too%s", args[0], hint)
		}
		return "", "", nil, cErr
	}
	echoMachineSource(ref, src)
	mID, mErr := resolveMachine(client, ref)
	if mErr != nil {
		return "", "", nil, mErr
	}
	pID, pErr := resolvePod(client, mID, args[0])
	if pErr != nil {
		return "", "", nil, pErr
	}
	return mID, pID, args[1:], nil
}

// resolveFirstArg rewrites args[0] from a machine name (or the ambient
// context, when no argument was given) into a machine id, and returns the
// adjusted slice.
//
// Commands predating name resolution take the machine as a raw UUID in
// args[0]. Rather than restructure each of them, they call this once after
// building the client: it keeps their existing args[0] indexing intact while
// making every one of them accept a name, a prefix, or -m / $USECTL_MACHINE /
// `usectl use`.
func resolveFirstArg(client *api.Client, args []string) ([]string, error) {
	given := ""
	if len(args) > 0 {
		given = args[0]
	}
	ref, src, err := machineRef(given)
	if err != nil {
		return nil, err
	}
	id, err := resolveMachine(client, ref)
	if err != nil {
		return nil, err
	}
	if src == srcArg {
		out := append([]string{}, args...)
		out[0] = id
		return out, nil
	}
	// The machine came from context, so it is not in args at all — prepend it
	// and every existing args[N] index still refers to what the command
	// expects.
	echoMachineSource(ref, src)
	return append([]string{id}, args...), nil
}

// resolveMachineOptionalPod handles commands that are machine-scoped but where
// naming a pod is a useful narrowing:
//
//	<machine> <pod>   both explicit
//	<machine>         machine only
//	<pod>             machine from context
//	(nothing)         machine from context
//
// The single-argument case is genuinely ambiguous, so it is resolved by trying
// the machine reading first and only treating the argument as a pod when that
// fails and a context machine exists. podID is "" when no pod was named.
func resolveMachineOptionalPod(client *api.Client, args []string) (machineID, podID string, err error) {
	switch len(args) {
	case 0:
		ref, src, cErr := machineRef("")
		if cErr != nil {
			return "", "", cErr
		}
		echoMachineSource(ref, src)
		machineID, err = resolveMachine(client, ref)
		return machineID, "", err
	case 1:
		if id, mErr := resolveMachine(client, args[0]); mErr == nil {
			return id, "", nil
		}
		ref, src, cErr := machineRef("")
		if cErr != nil {
			// No context to fall back on, so the argument really was meant to
			// be a machine; report that failure rather than a context hint.
			_, mErr := resolveMachine(client, args[0])
			return "", "", mErr
		}
		echoMachineSource(ref, src)
		machineID, err = resolveMachine(client, ref)
		if err != nil {
			return "", "", err
		}
		podID, err = resolvePod(client, machineID, args[0])
		return machineID, podID, err
	default:
		if mID, mErr := resolveMachine(client, args[0]); mErr == nil {
			if pID, pErr := resolvePod(client, mID, args[1]); pErr == nil {
				return mID, pID, nil
			}
		}
		// Reversed order, as in resolveMachineAndPod.
		if mID, mErr := resolveMachine(client, args[1]); mErr == nil {
			if pID, pErr := resolvePod(client, mID, args[0]); pErr == nil {
				return mID, pID, nil
			}
		}
		machineID, err = resolveMachine(client, args[0])
		if err != nil {
			return "", "", err
		}
		podID, err = resolvePod(client, machineID, args[1])
		return machineID, podID, err
	}
}

// resolveMachineAndAddon is the addon counterpart of resolveMachineAndPod:
//
//	<machine> <addon> [rest...]   explicit
//	<addon> [rest...]             machine from -m / $USECTL_MACHINE / `usectl use`
//
// An addon may be named by instance name, type, or "type/name". As with pods,
// the explicit reading is tried first and only abandoned when it does not
// resolve, so an addon that happens to share a machine's name never shadows it.
func resolveMachineAndAddon(client *api.Client, args []string) (machineID, addonID string, rest []string, err error) {
	if len(args) >= 2 {
		if mID, mErr := resolveMachine(client, args[0]); mErr == nil {
			if aID, aErr := resolveAddon(client, mID, args[1]); aErr == nil {
				return mID, aID, args[2:], nil
			}
		}
		// Reversed order, as in resolveMachineAndPod.
		if mID, mErr := resolveMachine(client, args[1]); mErr == nil {
			if aID, aErr := resolveAddon(client, mID, args[0]); aErr == nil {
				return mID, aID, args[2:], nil
			}
		}
	}
	if len(args) == 0 {
		return "", "", nil, fmt.Errorf("an addon is required")
	}
	ref, src, cErr := machineRef("")
	if cErr != nil {
		if len(args) >= 2 {
			if mID, mErr := resolveMachine(client, args[0]); mErr == nil {
				_, aErr := resolveAddon(client, mID, args[1])
				return "", "", nil, aErr
			}
			if mID, mErr := resolveMachine(client, args[1]); mErr == nil {
				_, aErr := resolveAddon(client, mID, args[0])
				return "", "", nil, aErr
			}
			return "", "", nil, fmt.Errorf("neither %q nor %q is a machine — run 'usectl machines list'", args[0], args[1])
		}
		if mID, mErr := resolveMachine(client, args[0]); mErr == nil {
			labels := []string{}
			if addons, lErr := client.ListProjectAddons(mID); lErr == nil {
				for _, a := range addons {
					labels = append(labels, a.AddonType+"/"+a.Name)
				}
				sort.Strings(labels)
			}
			hint := ""
			if len(labels) > 0 {
				hint = " — addons here: " + strings.Join(labels, ", ")
			}
			return "", "", nil, fmt.Errorf("%q is a machine, not an addon; name the addon too%s", args[0], hint)
		}
		return "", "", nil, cErr
	}
	echoMachineSource(ref, src)
	machineID, err = resolveMachine(client, ref)
	if err != nil {
		return "", "", nil, err
	}
	addonID, err = resolveAddon(client, machineID, args[0])
	if err != nil {
		return "", "", nil, err
	}
	return machineID, addonID, args[1:], nil
}

// resolveDeployment turns a deployment id or unambiguous id prefix into a full
// id.
//
// Needed because the listings print a shortened id for readability — copying
// what is on screen into build-logs or rollback otherwise fails with "invalid
// deployment id". Printing an identifier the next command rejects is a bug in
// the listing, not in the user's typing.
func resolveDeployment(client *api.Client, machineID, ref string) (string, error) {
	if ref == "" {
		return "", fmt.Errorf("a deployment id is required")
	}
	if isUUID(ref) {
		return ref, nil
	}
	// Scan a generous window: prefixes are usually copied from a recent row,
	// but "recent" is relative on a busy machine.
	page, err := client.ListDeployments(machineID, "", "", 1, 100)
	if err != nil {
		return "", err
	}
	var matches []string
	for _, d := range page.Deployments {
		if strings.HasPrefix(d.ID, ref) {
			matches = append(matches, d.ID)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("no deployment starting with %q in the last 100 — run 'usectl machines deployments <machine>'", ref)
	default:
		return "", ambiguousError("deployment", ref, matches)
	}
}
