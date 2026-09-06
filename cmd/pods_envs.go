package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

// `usectl machines pods env` — the environment a single pod actually receives.
//
// Environment reaches a pod from three places, and looking at any one of them
// alone answers the wrong question:
//
//	machine-level custom vars   `machines envs` — shared by every pod
//	per-pod overrides           set here
//	addon injections            DATABASE_URL & co, from ATTACHED addons only
//
// `machines envs <machine>` shows only the first, which is why looking there
// for a pod's variables comes up short. This command shows the pod's own
// overrides and points at the other two sources.

var podsEnvCmd = &cobra.Command{
	Use:     "env [machine] <pod>",
	Aliases: []string{"envs", "variables", "vars"},
	Short:   "Show or set the environment variables of one pod",
	Long: `List a pod's own environment variables, and where the rest of its environment
comes from.

A pod's runtime environment is the union of three sources:

  1. machine-wide variables   usectl machines envs <machine>
  2. this pod's overrides     shown here
  3. attached addons          usectl machines pods addons <machine> <pod>

Addon credentials (DATABASE_URL, REDIS_URL, …) are injected only for addons
ATTACHED to this pod — a machine can own a database that a given pod cannot
see.`,
	Example: `  usectl machines pods env api web
  usectl machines pods env api web LOG_LEVEL=debug
  usectl machines pods env web              # with a default machine set`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID, podID, pairs, err := resolveMachineAndPod(client, args)
		if err != nil {
			return err
		}

		if len(pairs) > 0 {
			vars := map[string]string{}
			for _, p := range pairs {
				k, v, _ := strings.Cut(p, "=")
				vars[k] = v
			}
			if err := client.UpdateAppEnvs(machineID, podID, vars); err != nil {
				return err
			}
			fmt.Printf("✓ Set %d variable(s) on this pod. Takes effect on the next deploy.\n", len(vars))
			return nil
		}

		return printPodEnv(client, machineID, podID)
	},
}

// printPodEnv renders everything one pod receives.
//
// The endpoint returns the MERGED environment with a per-key source, so
// machine-wide values, pod overrides and addon injections are distinguishable
// in one listing instead of three commands.
func printPodEnv(client *api.Client, machineID, podID string) error {
	resp, err := client.ListAppEnvs(machineID, podID)
	if err != nil {
		return err
	}

	// The endpoint merges only the project and app vaults (sources "project"
	// and "app"); it does not know about addon-injected variables at all. Yet
	// DATABASE_URL, REDIS_URL and the rest genuinely reach the container, via
	// EnvFrom on the addon's Secret. Reporting only what the endpoint returns
	// therefore tells the user that nothing reaches a pod that is in fact
	// fully wired up, so the addon keys are folded in here.
	addonVars := podAddonVars(client, machineID, podID)

	if jsonOutput {
		return output.JSON(struct {
			Vars  []api.AppEnvVarEntry `json:"vars"`
			Addon []addonVar           `json:"addon_vars"`
		}{resp.Vars, addonVars})
	}
	if (resp == nil || len(resp.Vars) == 0) && len(addonVars) == 0 {
		fmt.Println("No variables reach this pod yet.")
		fmt.Println(output.Dim("  set one:      usectl machines envs <machine> <pod> KEY=value"))
		fmt.Println(output.Dim("  attach one:   usectl machines pods attach-addon <machine> <pod> database"))
		fmt.Println(output.Dim("  see what an addon injects before attaching:  usectl machines addons catalog"))
		return nil
	}

	// The endpoint already returns the MERGED environment with a per-key
	// source, so machine-wide values, pod overrides and addon injections
	// are distinguishable in one listing rather than three commands.
	rows := make([][]string, 0, len(resp.Vars))
	for _, v := range resp.Vars {
		val := "—"
		switch {
		case v.Protected:
			// Value is JSON null for protected keys — deliberately
			// distinct from "set to empty".
			val = output.Dim("(protected)")
		case v.Value != nil:
			val = *v.Value
			// DATABASE_URL and friends are live credentials. A listing command
			// lands in scrollback, screenshots and CI logs, so they are masked
			// unless explicitly revealed — the same rule the addon view uses.
			if !envReveal && secretKey(v.Key, val) {
				val = output.Dim(maskValue(val, false))
			}
		}
		src := v.Source
		if v.Overridden {
			src += output.Yellow(" (overridden)")
		}
		rows = append(rows, []string{output.Cyan(v.Key), val, src})
	}
	for _, av := range addonVars {
		val := av.Value
		if !envReveal && secretKey(av.Key, val) {
			val = output.Dim(maskValue(val, false))
		}
		rows = append(rows, []string{output.Cyan(av.Key), val, "addon:" + av.Addon})
	}
	output.Table([]string{"KEY", "VALUE", "SOURCE"}, rows)
	fmt.Println()
	fmt.Println(output.Dim("source: 'app' is this pod's own override, 'project' is machine-wide,"))
	fmt.Println(output.Dim("        'addon:<type/name>' is injected from an attached addon."))
	if !envReveal {
		fmt.Println(output.Dim("secrets masked — pass --reveal to show"))
	}
	return nil
}

// printMachineEnv renders the machine-wide custom variables.
func printMachineEnv(client *api.Client, machineID string) error {
	envs, err := client.ListEnvs(machineID)
	if err != nil {
		return err
	}
	if jsonOutput {
		return output.JSON(envs)
	}
	if len(envs) == 0 {
		fmt.Println("No machine-wide variables set.")
		fmt.Println(output.Dim("  set one:  usectl machines envs <machine> KEY=value"))
		fmt.Println(output.Dim("  a pod also receives addon credentials:  usectl machines envs <machine> <pod>"))
		return nil
	}
	keys := make([]string, 0, len(envs))
	for k := range envs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	rows := make([][]string, len(keys))
	for i, k := range keys {
		ptr := envs[k]
		// nil means protected: the API refuses to return it. Never print a
		// partial preview — this output lands in terminals and CI logs.
		if ptr == nil {
			rows[i] = []string{output.Cyan(k), output.Dim("(protected)")}
			continue
		}
		v := *ptr
		switch {
		case !envReveal && secretKey(k, v):
			v = output.Dim(maskValue(v, false))
		case len(v) > 48:
			// Truncation is for readability only; it must never stand in for
			// masking, since a short secret survives it intact.
			v = v[:24] + "…" + v[len(v)-8:]
		}
		rows[i] = []string{output.Cyan(k), v}
	}
	output.Table([]string{"KEY", "VALUE"}, rows)
	fmt.Println()
	fmt.Println(output.Dim("machine-wide only — a pod also receives its own overrides and attached addon credentials:"))
	fmt.Println(output.Dim("  usectl machines envs <machine> <pod>"))
	return nil
}

// envReveal is bound to --reveal on the env-listing commands.
var envReveal bool

func init() {
	podsEnvCmd.Flags().BoolVar(&envReveal, "reveal", false, "Show secret values in clear text")
	envsCmd.Flags().BoolVar(&envReveal, "reveal", false, "Show secret values in clear text")
}

// addonVar is one variable an addon injects into a pod.
type addonVar struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Addon string `json:"addon"`
}

// podAddonVars returns the variables the addons attached to this pod inject.
//
// It mirrors deployer.perAppAddonSecrets, including its fallback: a pod with NO
// attachment rows receives EVERY addon in the machine (deployer.go:1026). Only
// listing explicit attachments would under-report exactly the pods that are
// wired up by default.
//
// Keys prefixed with "_" are platform metadata, not variables the container
// sees, so they are excluded.
func podAddonVars(client *api.Client, machineID, podID string) []addonVar {
	all, err := client.ListProjectAddons(machineID)
	if err != nil || len(all) == 0 {
		return nil
	}
	effective := all
	if attached, aErr := client.ListAppAddonAttachments(machineID, podID); aErr == nil && len(attached) > 0 {
		effective = attached
	}

	out := []addonVar{}
	for _, a := range effective {
		label := a.AddonType + "/" + a.Name
		prefix := addonEnvPrefix(&a)
		for k, v := range a.Config {
			bare := k
			if prefix != "" {
				bare = strings.TrimPrefix(k, prefix)
			}
			if strings.HasPrefix(bare, "_") {
				continue
			}
			out = append(out, addonVar{Key: k, Value: v, Addon: label})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
