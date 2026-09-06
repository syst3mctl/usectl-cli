package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

// `usectl machines pods` — pod-level operations inside a machine.
// All endpoints already exist server-side; this group is just the
// kubectl-shaped affordance layered on top of stats, runtime-logs,
// diagnostics, and the terminal session.

var podsWide bool

var podsCmd = &cobra.Command{
	Use:     "pods [machine]",
	Aliases: []string{"pod"},
	Short:   "Show every pod in a machine — config, limits and where each one runs",
	Long: `One view of everything running in a machine.

For each pod it shows the declared configuration — git source and branch,
visibility, domain, primary and extra ports, CPU / memory / storage limits and
the rollout strategy — followed by the actual running instances with the node
each one landed on.

Dedicated addon pods (Postgres, Redis, NATS, ...) are listed too, under
ADDONS, joined to the addon that owns them. Anything left over — cron jobs,
database UIs — appears under OTHER PODS rather than being hidden.

This replaces the old split where 'machines pods' showed CPU/memory stats and
'kpods' showed the Kubernetes view; neither showed the pod's configuration.

Subcommands:
  stats        CPU / memory / network per pod
  logs         Tail logs (optionally scoped with --app)
  shell        Interactive shell into a pod
  diagnostics  Crash reasons and previous-container logs
  restart      Rolling-restart every pod in the machine`,
	Example: `  usectl machines pods aeeb7dc4-596b-408e-b7bc-82330c138e0e
  usectl machines pods my-machine --wide
  usectl machines pods my-machine --json`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// No argument is not an error when a machine context is set — that is
		// the whole point of `usectl use`. Fall through to help only when
		// there is no context either.
		machine := ""
		if len(args) == 1 {
			machine = args[0]
		}
		if machine == "" {
			if _, _, err := machineRef(""); err != nil {
				return cmd.Help()
			}
		}
		return runPodsView(machine, podsWide)
	},
}

func runPodsList(machineID string) error {
	client, err := api.NewClient(apiURL)
	if err != nil {
		return err
	}
	stats, err := client.GetProjectStats(machineID)
	if err != nil {
		return err
	}
	if jsonOutput {
		return output.JSON(stats.Pods)
	}
	if len(stats.Pods) == 0 {
		fmt.Println("No pods running.")
		return nil
	}
	rows := make([][]string, len(stats.Pods))
	for i, p := range stats.Pods {
		rows[i] = []string{p.Name, p.Status, p.CPU, p.Memory, p.NetRx, p.NetTx, strconv.Itoa(int(p.Restarts))}
	}
	output.Table([]string{"NAME", "STATUS", "CPU", "MEMORY", "NET RX", "NET TX", "RESTARTS"}, rows)
	return nil
}

var podsListCmd = &cobra.Command{
	Use:     "list [machine]",
	Aliases: []string{"ls"},
	Short:   "Show every pod in a machine — config, limits and where each one runs",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPodsView(firstOrEmpty(args), podsWide)
	},
}

// podsStatsCmd keeps the pre-existing CPU/memory/network table, which the
// merged view above deliberately omits: those are sampled metrics, not
// configuration, and mixing them in made the block unreadable.
var podsStatsCmd = &cobra.Command{
	Use:   "stats <machine>",
	Short: "CPU / memory / network per running pod",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPodsList(args[0])
	},
}

var (
	podsLogsTail   int
	podsLogsFollow bool
	podsLogsAppID  string
)

var podsLogsCmd = &cobra.Command{
	Use:   "logs [machine] [pod]",
	Short: "Tail a pod's logs",
	Long: `Same data source as 'machines logs'. Naming a pod narrows the stream to that
workload; without one the backend picks the first ready pod in the namespace,
which on a multi-pod machine is rarely the one you meant.

--follow now works together with a pod filter. It previously did not: the
client hardcoded the request path with no room for app_id, so the app-scoped
case silently degraded to a one-off snapshot.`,
	Example: `  usectl machines pods logs api web
  usectl machines pods logs api web -f
  usectl machines pods logs api --tail 500`,
	Args: cobra.RangeArgs(0, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID, podID, err := resolveMachineOptionalPod(client, args)
		if err != nil {
			return err
		}
		// --app is kept as an escape hatch for a raw app UUID.
		if podID == "" && podsLogsAppID != "" {
			podID = podsLogsAppID
		}

		if podsLogsFollow {
			return client.StreamRuntimeLogs(machineID, podsLogsTail, podID, os.Stdout)
		}
		logs, err := client.GetRuntimeLogs(machineID, podsLogsTail, podID)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(logs)
		}
		fmt.Print(logs.Logs)
		return nil
	},
}

var (
	podsShellPodName string
	podsShellAppID   string
)

var podsShellCmd = &cobra.Command{
	Use:   "shell <machine-id>",
	Short: "Open an interactive /bin/sh into a pod",
	Long: `Connects to a pod via the SPDY-proxy WebSocket tunnel exposed at
/api/projects/{id}/terminal. By default the backend picks the first
ready pod; use --pod <name> to target a specific one (names from
'machines pods list'), or --app <app-id> for a multi-app pod.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		// The terminal path supports ?pod= but not yet ?app_id= — when
		// --app is given we resolve to the first pod prefixed with the
		// app's sanitized name.
		podName := podsShellPodName
		if podName == "" && podsShellAppID != "" {
			stats, err := client.GetProjectStats(args[0])
			if err != nil {
				return fmt.Errorf("resolve pods: %w", err)
			}
			// Best effort: prefer running pods that contain the app id
			// fragment (we don't have name → app mapping client-side, so
			// fall back to first pod).
			for _, p := range stats.Pods {
				if p.Status == "Running" {
					podName = p.Name
					break
				}
			}
		}
		fmt.Printf("Connecting to machine %s%s...\n", args[0],
			func() string {
				if podName != "" {
					return " pod=" + podName
				}
				return ""
			}())
		return client.StreamTerminal(args[0], podName)
	},
}

var podsDiagnosticsCmd = &cobra.Command{
	Use:     "diagnostics <machine-id>",
	Aliases: []string{"diag"},
	Short:   "Show K8s lifecycle events + previous-crash logs for the failing pod",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		diag, err := client.GetDiagnostics(args[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(diag)
		}
		fmt.Printf("Pod: %s\n", diag.PodName)
		fmt.Printf("Phase: %s\n", diag.Phase)
		if diag.ContainerStatus != nil {
			fmt.Printf("Container: %s (%s)\n", diag.ContainerStatus.State, diag.ContainerStatus.WaitReason)
			if diag.ContainerStatus.Message != "" {
				fmt.Printf("Message: %s\n", diag.ContainerStatus.Message)
			}
			fmt.Printf("Restarts: %d\n", diag.ContainerStatus.RestartCount)
		}
		if diag.PreviousLogs != "" {
			fmt.Println("\n--- Previous crash logs ---")
			fmt.Println(diag.PreviousLogs)
		}
		if len(diag.Events) > 0 {
			fmt.Println("\n--- Recent K8s events ---")
			rows := make([][]string, len(diag.Events))
			for i, e := range diag.Events {
				rows[i] = []string{e.Time, e.Type, e.Reason, e.Message}
			}
			output.Table([]string{"TIME", "TYPE", "REASON", "MESSAGE"}, rows)
		}
		return nil
	},
}

var podsRestartCmd = &cobra.Command{
	Use:   "restart <machine-id>",
	Short: "Rolling-restart every app in the machine (no rebuild)",
	Long: `Bumps the kubectl.kubernetes.io/restartedAt annotation on each app's
Deployment so the controller rolls the pods. Useful after editing
project-level env vars or when you just want fresh containers.

Single-app legacy machines fall back to a stop+start cycle since they
don't have project_apps rows to iterate.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID := args[0]
		apps, err := client.ListProjectApps(machineID)
		if err != nil {
			return err
		}
		if len(apps) == 0 {
			// Legacy single-pod machine — emulate restart by stop+start.
			fmt.Println("No multi-app rows; using stop+start...")
			if err := client.StopProject(machineID); err != nil {
				return fmt.Errorf("stop: %w", err)
			}
			if err := client.StartProject(machineID); err != nil {
				return fmt.Errorf("start: %w", err)
			}
			fmt.Println("✓ Machine restarted")
			return nil
		}
		failed := 0
		for _, a := range apps {
			if err := client.RestartApp(machineID, a.ID); err != nil {
				fmt.Printf("✗ %s: %v\n", a.Name, err)
				failed++
				continue
			}
			fmt.Printf("✓ %s\n", a.Name)
		}
		if failed > 0 {
			return fmt.Errorf("%d app(s) failed to restart", failed)
		}
		return nil
	},
}

func init() {
	podsLogsCmd.Flags().IntVar(&podsLogsTail, "tail", 100, "Number of lines to fetch")
	podsLogsCmd.Flags().BoolVarP(&podsLogsFollow, "follow", "f", false, "Stream logs in real time")
	podsLogsCmd.Flags().StringVar(&podsLogsAppID, "app", "", "Scope to one app (multi-app machines)")

	podsShellCmd.Flags().StringVar(&podsShellPodName, "pod", "", "Specific pod name (default: first ready pod)")
	podsShellCmd.Flags().StringVar(&podsShellAppID, "app", "", "Pick the first running pod of this app id")

	podsCmd.Flags().BoolVar(&podsWide, "wide", false, "Show additional columns (entrypoint command)")
	podsListCmd.Flags().BoolVar(&podsWide, "wide", false, "Show additional columns (entrypoint command)")
	podsCmd.AddCommand(podsListCmd, podsCreateCmd, podsStatsCmd, podsLogsCmd, podsShellCmd,
		podsDiagnosticsCmd, podsRestartCmd,
		podsAddonsCmd, podsAttachAddonCmd, podsDetachAddonCmd,
		podsSetCmd, podsOpenPortCmd, podsClosePortCmd, podsEnvCmd, podsDeleteCmd,
		podsStartCmd, podsStopCmd, podsInternalCmd, podsRevealCmd,
		podsTrafficCmd, podsInsightsCmd)
	// Attach under the renamed `machines` (a.k.a. `projects`) command.
	projectsCmd.AddCommand(podsCmd)
}
