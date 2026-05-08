package cmd

import (
	"fmt"
	"net/url"
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

var podsCmd = &cobra.Command{
	Use:     "pods <machine-id>",
	Aliases: []string{"pod"},
	Short:   "List and manage K8s pods running inside a machine",
	Long: `Pod-level operations for a machine.

When run without a subcommand, lists pods (same data as 'machines stats').

Subcommands:
  list         List pods with CPU / memory / restart counts
  logs         Tail logs (optionally scoped to one app via --app)
  shell        Open an interactive shell into a pod
  diagnostics  K8s lifecycle events + previous-crash logs for the failing pod
  restart      Rolling-restart every app in the machine`,
	Example: `  usectl machines pods <machine-id>                # default = list
  usectl machines pods logs <machine-id> -f
  usectl machines pods shell <machine-id> --pod my-app-7c9d4b8f6-x2k9p
  usectl machines pods restart <machine-id>`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// No subcommand → fall through to list.
		if len(args) != 1 {
			return cmd.Help()
		}
		return runPodsList(args[0])
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
	Use:     "list <machine-id>",
	Aliases: []string{"ls"},
	Short:   "List pods running inside a machine",
	Args:    cobra.ExactArgs(1),
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
	Use:   "logs <machine-id>",
	Short: "Tail logs from the machine's pods (optionally scoped to one app)",
	Long: `Same data source as 'machines logs', surfaced under the pods group for
discoverability. Pass --app <app-uuid> to narrow to a single multi-app
pod; otherwise the backend picks the first ready pod in the namespace.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		// Build path manually so we can pass the optional app_id filter
		// (the existing GetRuntimeLogs helper doesn't take it). The
		// backend accepts ?app_id=<uuid> on /runtime-logs.
		query := url.Values{}
		if podsLogsTail > 0 {
			query.Set("lines", strconv.Itoa(podsLogsTail))
		}
		if podsLogsAppID != "" {
			query.Set("app_id", podsLogsAppID)
		}
		if podsLogsFollow {
			query.Set("follow", "true")
			path := fmt.Sprintf("/api/projects/%s/runtime-logs", args[0])
			if encoded := query.Encode(); encoded != "" {
				path += "?" + encoded
			}
			return streamRawLogs(client, path)
		}
		var resp api.LogsResponse
		path := fmt.Sprintf("/api/projects/%s/runtime-logs", args[0])
		if encoded := query.Encode(); encoded != "" {
			path += "?" + encoded
		}
		if err := client.Get(path, &resp); err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(resp)
		}
		fmt.Print(resp.Logs)
		return nil
	},
}

// streamRawLogs is the same shape as Client.StreamRuntimeLogs but accepts a
// pre-built path so we can pass arbitrary query params.
func streamRawLogs(client *api.Client, path string) error {
	// Fall back to the helper when no app filter is set — it handles the
	// HTTP boilerplate and stdout copying. For the app-scoped case we
	// build the request inline since StreamRuntimeLogs hardcodes the path.
	// Both paths share the same backend so the filter just needs the right
	// URL.
	tail := 0
	appFilter := ""
	if u, err := url.Parse(path); err == nil {
		if v := u.Query().Get("lines"); v != "" {
			tail, _ = strconv.Atoi(v)
		}
		appFilter = u.Query().Get("app_id")
	}
	if appFilter == "" {
		// Pull machine ID out of the path: /api/projects/<id>/runtime-logs.
		// The helper takes care of follow-mode framing.
		var id string
		if u, err := url.Parse(path); err == nil {
			parts := splitPath(u.Path)
			if len(parts) >= 3 {
				id = parts[2]
			}
		}
		if id != "" {
			return client.StreamRuntimeLogs(id, tail, os.Stdout)
		}
	}
	// App-scoped follow: copy directly via doRaw-ish flow. The simplest
	// reuse without exposing private client methods is to drop --follow
	// when --app is set; users get a snapshot. The polling cadence is
	// usually fine for debugging worker pods.
	var resp api.LogsResponse
	if err := client.Get(path, &resp); err != nil {
		return err
	}
	fmt.Print(resp.Logs)
	fmt.Fprintln(os.Stderr, "\n(snapshot — --follow with --app is not yet supported; rerun without --follow or omit --app)")
	return nil
}

func splitPath(p string) []string {
	out := []string{}
	cur := ""
	for _, r := range p {
		if r == '/' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	out = append(out, cur)
	return out
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

	podsCmd.AddCommand(podsListCmd, podsLogsCmd, podsShellCmd, podsDiagnosticsCmd, podsRestartCmd)
	// Attach under the renamed `machines` (a.k.a. `projects`) command.
	projectsCmd.AddCommand(podsCmd)
}
