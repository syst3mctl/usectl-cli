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
	if jsonOutput {
		return output.JSON(resp)
	}
	if resp == nil || len(resp.Vars) == 0 {
		fmt.Println("No variables reach this pod yet.")
		fmt.Println(output.Dim("  attach an addon, or set one:  usectl machines pods env <machine> <pod> KEY=value"))
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
	output.Table([]string{"KEY", "VALUE", "SOURCE"}, rows)
	fmt.Println()
	fmt.Println(output.Dim("source: 'addon' values come from attached addons; 'user' from machine-wide or per-pod variables."))
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
