package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

// parseMemoryToMiB accepts human-friendly memory strings and returns mebibytes.
// Forms accepted:
//
//	"1024"   → 1024 MiB (bare integer treated as MiB)
//	"512Mi"  → 512 MiB
//	"512M"   → 488 MiB (decimal megabytes; 512*1000*1000 / 2^20, rounded down)
//	"1Gi"    → 1024 MiB
//	"2G"     → 1907 MiB (decimal gigabytes)
//
// Empty string returns (0, nil) so callers can distinguish "not set" from "0".
// Anything else is a parse error.
func parseMemoryToMiB(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	// Strip trailing "B" if present so "MiB"/"GiB"/"MB"/"GB" all work.
	if strings.HasSuffix(s, "B") || strings.HasSuffix(s, "b") {
		s = s[:len(s)-1]
	}
	var unit string
	var numStr string
	switch {
	case strings.HasSuffix(s, "Mi") || strings.HasSuffix(s, "mi"):
		unit = "Mi"
		numStr = s[:len(s)-2]
	case strings.HasSuffix(s, "Gi") || strings.HasSuffix(s, "gi"):
		unit = "Gi"
		numStr = s[:len(s)-2]
	case strings.HasSuffix(s, "M") || strings.HasSuffix(s, "m"):
		unit = "M"
		numStr = s[:len(s)-1]
	case strings.HasSuffix(s, "G") || strings.HasSuffix(s, "g"):
		unit = "G"
		numStr = s[:len(s)-1]
	default:
		unit = ""
		numStr = s
	}
	n, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory %q: %w", s, err)
	}
	if n < 0 {
		return 0, fmt.Errorf("invalid memory %q: must be >= 0", s)
	}
	switch unit {
	case "", "Mi":
		return int(n), nil
	case "Gi":
		return int(n * 1024), nil
	case "M":
		return int(n * 1000 * 1000 / (1024 * 1024)), nil
	case "G":
		return int(n * 1000 * 1000 * 1000 / (1024 * 1024)), nil
	}
	return 0, fmt.Errorf("unknown memory unit in %q", s)
}

// parseCPUToMillis accepts human-friendly CPU strings and returns millicores.
// Forms accepted:
//
//	"500"    → 500 m  (bare integer treated as millicores)
//	"500m"   → 500 m
//	"1"      → 1000 m (no suffix => whole-core float)... wait, ambiguity.
//
// To avoid the "is '1' one core or one millicore?" trap, the rule is:
//
//	bare integer or float WITHOUT 'm' suffix → whole cores (1 = 1000m, 0.5 = 500m)
//	with 'm' suffix → millicores
//
// This matches kubectl's behavior. Empty string returns (0, nil).
func parseCPUToMillis(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	if strings.HasSuffix(s, "m") || strings.HasSuffix(s, "M") {
		n, err := strconv.Atoi(s[:len(s)-1])
		if err != nil {
			return 0, fmt.Errorf("invalid cpu %q: %w", s, err)
		}
		if n < 0 {
			return 0, fmt.Errorf("invalid cpu %q: must be >= 0", s)
		}
		return n, nil
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid cpu %q: %w", s, err)
	}
	if n < 0 {
		return 0, fmt.Errorf("invalid cpu %q: must be >= 0", s)
	}
	return int(n * 1000), nil
}

var appsCmd = &cobra.Command{
	Use:     "apps",
	Aliases: []string{"app"},
	Short:   "Manage multi-app pods inside a project (web/worker/release)",
	Long: `Each project can host multiple "apps" — independent pods built from their
own GitHub repo + branch. Use this to run a frontend + backend + worker in
the same project namespace, all sharing the same addons (database, redis, etc).

When to add an app vs. create a new machine:
  - Same product, different roles (frontend + API + worker)  → one machine,
    multiple apps. They share the namespace, the addons, and the DB.
  - Unrelated products that should not share credentials       → separate
    machines. NetworkPolicies block cross-machine traffic.

App kinds (--kind):
  web      Default. Public Service + IngressRoute. The machine's domain
           and the app's --domain route traffic to its --port.
  worker   No Service, no ingress. Runs --command as a long-lived process
           (queue consumers, background jobs). Still gets addon env vars.
  release  One-shot Job that runs to completion before each 'web' rollout.
           Use for DB migrations or cache warmups. Failure blocks the deploy.

Build rules per app are the same as for the parent machine — see
'usectl machines create --help' for Dockerfile / auto-detect details.
Each app can override the machine's env vars via 'usectl apps envs'.

Subcommands:
  list        List apps in a project
  create      Add a new app to a machine
  update      Change an app's branch, port, replicas, visibility, etc.
  delete      Delete an app and its K8s resources
  start/stop  Scale an app's deployment to 0 / restore replicas
  restart     Rolling restart (no rebuild) — picks up new env vars
  internal    Show the cluster-internal address for app-to-app calls
  variables   Show resolved env vars (user + addon-injected)
  reveal      Reveal a masked variable value
  envs        Manage per-app env vars (overrides project envs)
  addons      Attach / detach addons to a single app
  traffic     Show Traefik request metrics
  insights    Show per-pod CPU/memory history + recent error logs`,
}

var appsListCmd = &cobra.Command{
	Use:   "list <project-id>",
	Short: "List all apps in a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		apps, err := client.ListProjectApps(args[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(apps)
		}
		if len(apps) == 0 {
			fmt.Println("No apps in this project.")
			return nil
		}
		rows := make([][]string, len(apps))
		for i, a := range apps {
			vis := "public"
			if !a.IsPublic {
				vis = "private"
			}
			state := "running"
			if a.IsStopped {
				state = "stopped"
			}
			rows[i] = []string{
				a.ID, a.Name, a.Kind, a.Branch,
				strconv.Itoa(a.Port), strconv.Itoa(a.Replicas),
				vis, state, a.Domain,
			}
		}
		output.Table(
			[]string{"ID", "NAME", "KIND", "BRANCH", "PORT", "REPLICAS", "VISIBILITY", "STATE", "DOMAIN"},
			rows,
		)
		return nil
	},
}

var (
	appCreateName        string
	appCreateRepo        string
	appCreateBranch      string
	appCreateDomain      string
	appCreatePort        int
	appCreateReplicas    int
	appCreateInstallID   int64
	appCreateAutoDeploy  bool
	appCreatePreviewEnvs bool
	appCreatePrivate     bool
	appCreateKind        string
	appCreateCommand     string
	appCreateArgs        []string
)

var appsCreateCmd = &cobra.Command{
	Use:   "create <project-id>",
	Short: "Add a new app pod to an existing project",
	Long: `Create a web/worker/release pod inside an existing project. Each app
gets its own GitHub repo + branch but shares the machine's addons and
networking.`,
	Example: `  # Web app
  usectl apps create proj-id --name api --repo https://github.com/me/api --port 8080

  # Worker pod (no Service, no IngressRoute — runs a long-lived command)
  usectl apps create proj-id --name worker --repo https://github.com/me/api \
    --kind worker --command "node worker.js"

  # Private app (cluster-internal only)
  usectl apps create proj-id --name auth --repo https://github.com/me/auth --private`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		req := api.CreateProjectAppRequest{
			Name:              appCreateName,
			RepoURL:           appCreateRepo,
			Branch:            appCreateBranch,
			Domain:            appCreateDomain,
			Port:              appCreatePort,
			Replicas:          appCreateReplicas,
			AutoDeploy:        appCreateAutoDeploy,
			EnablePreviewEnvs: appCreatePreviewEnvs,
			Kind:              appCreateKind,
			Command:           appCreateCommand,
			Args:              appCreateArgs,
		}
		if appCreateInstallID > 0 {
			req.InstallationID = &appCreateInstallID
		}
		if cmd.Flags().Changed("private") {
			isPublic := !appCreatePrivate
			req.IsPublic = &isPublic
		}
		app, err := client.CreateProjectApp(args[0], req)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(app)
		}
		fmt.Printf("✓ App created: %s (ID: %s, kind: %s)\n", app.Name, app.ID, app.Kind)
		return nil
	},
}

var (
	appUpdateBranch    string
	appUpdateDomain    string
	appUpdatePort      int
	appUpdateReplicas  int
	appUpdateAutoDep   bool
	appUpdatePreview   bool
	appUpdatePrivate   bool
	appUpdateDotenvP   string
	appUpdateDotenvA   bool
	appUpdateKind      string
	appUpdateCommand   string
	appUpdateArgs      []string
)

var appsUpdateCmd = &cobra.Command{
	Use:   "update <project-id> <app-id>",
	Short: "Update settings on an existing app",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		req := api.UpdateProjectAppRequest{}
		if cmd.Flags().Changed("branch") {
			req.Branch = &appUpdateBranch
		}
		if cmd.Flags().Changed("domain") {
			req.Domain = &appUpdateDomain
		}
		if cmd.Flags().Changed("port") {
			req.Port = &appUpdatePort
		}
		if cmd.Flags().Changed("replicas") {
			req.Replicas = &appUpdateReplicas
		}
		if cmd.Flags().Changed("auto-deploy") {
			req.AutoDeploy = &appUpdateAutoDep
		}
		if cmd.Flags().Changed("preview-envs") {
			req.EnablePreviewEnvs = &appUpdatePreview
		}
		if cmd.Flags().Changed("private") {
			isPublic := !appUpdatePrivate
			req.IsPublic = &isPublic
		}
		if cmd.Flags().Changed("dotenv-path") {
			req.DotenvPath = &appUpdateDotenvP
		}
		if cmd.Flags().Changed("dotenv-auto") {
			req.DotenvAuto = &appUpdateDotenvA
		}
		if cmd.Flags().Changed("kind") {
			req.Kind = &appUpdateKind
		}
		if cmd.Flags().Changed("command") {
			req.Command = &appUpdateCommand
		}
		if cmd.Flags().Changed("args") {
			req.Args = appUpdateArgs
		}
		app, warning, err := client.UpdateProjectApp(args[0], args[1], req)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(app)
		}
		fmt.Printf("✓ App updated: %s\n", app.Name)
		if warning != "" {
			fmt.Printf("  ⚠ %s\n", warning)
		}
		return nil
	},
}

var appsDeleteCmd = &cobra.Command{
	Use:   "delete <project-id> <app-id>",
	Short: "Delete an app and tear down its K8s resources",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if err := client.DeleteProjectApp(args[0], args[1]); err != nil {
			return err
		}
		fmt.Println("✓ App deleted")
		return nil
	},
}

var appsStartCmd = &cobra.Command{
	Use:   "start <project-id> <app-id>",
	Short: "Start an app (restore replicas from 0)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if err := client.StartApp(args[0], args[1]); err != nil {
			return err
		}
		fmt.Println("✓ App started")
		return nil
	},
}

var appsStopCmd = &cobra.Command{
	Use:   "stop <project-id> <app-id>",
	Short: "Stop an app (scale Deployment to 0)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if err := client.StopApp(args[0], args[1]); err != nil {
			return err
		}
		fmt.Println("✓ App stopped")
		return nil
	},
}

var appsRestartCmd = &cobra.Command{
	Use:   "restart <project-id> <app-id>",
	Short: "Rolling restart (no rebuild) — picks up new env vars",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if err := client.RestartApp(args[0], args[1]); err != nil {
			return err
		}
		fmt.Println("✓ App restart triggered")
		return nil
	},
}

var appsInternalCmd = &cobra.Command{
	Use:   "internal <project-id> <app-id>",
	Short: "Show the cluster-internal address for app-to-app calls",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		addr, err := client.GetAppInternalAddress(args[0], args[1])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(addr)
		}
		output.Table([]string{"FIELD", "VALUE"}, [][]string{
			{"Service", addr.ServiceName},
			{"Namespace", addr.Namespace},
			{"Port", strconv.Itoa(addr.Port)},
			{"Short DNS", addr.ShortDNS},
			{"FQDN", addr.FQDN},
			{"URL (short)", addr.URLShort},
			{"URL (FQDN)", addr.URLFQDN},
		})
		return nil
	},
}

var appsVariablesCmd = &cobra.Command{
	Use:     "variables <project-id> <app-id>",
	Aliases: []string{"vars"},
	Short:   "Show resolved env vars (user + addon-injected, masked by default)",
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		resp, err := client.GetAppVariables(args[0], args[1])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(resp)
		}
		fmt.Println("User variables:")
		if len(resp.User) == 0 {
			fmt.Println("  (none)")
		} else {
			rows := make([][]string, len(resp.User))
			for i, e := range resp.User {
				rows[i] = []string{e.Key, e.Value}
			}
			output.Table([]string{"KEY", "VALUE"}, rows)
		}
		fmt.Println()
		fmt.Println("Addon-injected variables:")
		if len(resp.Addons) == 0 {
			fmt.Println("  (none)")
			return nil
		}
		rows := make([][]string, len(resp.Addons))
		for i, e := range resp.Addons {
			rows[i] = []string{e.Key, e.Value, e.AddonType}
		}
		output.Table([]string{"KEY", "VALUE", "ADDON"}, rows)
		return nil
	},
}

var appsRevealCmd = &cobra.Command{
	Use:   "reveal <project-id> <app-id> <key>",
	Short: "Reveal the unmasked value of a single variable (audited)",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		val, err := client.RevealAppVariable(args[0], args[1], args[2])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(map[string]string{"key": args[2], "value": val})
		}
		fmt.Println(val)
		return nil
	},
}

// --- per-app envs ---

var appsEnvsCmd = &cobra.Command{
	Use:   "envs",
	Short: "Manage per-app environment variables (override project envs)",
}

var appsEnvsListCmd = &cobra.Command{
	Use:   "list <project-id> <app-id>",
	Short: "List per-app + inherited project env vars",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		resp, err := client.ListAppEnvs(args[0], args[1])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(resp)
		}
		if len(resp.Vars) == 0 {
			fmt.Println("No env vars set.")
			return nil
		}
		rows := make([][]string, len(resp.Vars))
		for i, v := range resp.Vars {
			source := v.Source
			if v.Overridden {
				source += " (overridden)"
			}
			rows[i] = []string{v.Key, v.Value, source}
		}
		output.Table([]string{"KEY", "VALUE", "SOURCE"}, rows)
		return nil
	},
}

var appsEnvsSetCmd = &cobra.Command{
	Use:   "set <project-id> <app-id> KEY=VALUE [KEY=VALUE ...]",
	Short: "Set one or more per-app env vars",
	Args:  cobra.MinimumNArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		vars := map[string]string{}
		for _, kv := range args[2:] {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid format %q (expected KEY=VALUE)", kv)
			}
			vars[parts[0]] = parts[1]
		}
		if err := client.UpdateAppEnvs(args[0], args[1], vars); err != nil {
			return err
		}
		fmt.Printf("✓ Set %d env var(s) on app %s\n", len(vars), args[1])
		return nil
	},
}

var appsEnvsDeleteCmd = &cobra.Command{
	Use:   "delete <project-id> <app-id> KEY [KEY ...]",
	Short: "Delete one or more per-app env vars",
	Args:  cobra.MinimumNArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		keys := args[2:]
		if err := client.DeleteAppEnvs(args[0], args[1], keys); err != nil {
			return err
		}
		fmt.Printf("✓ Deleted %d key(s) on app %s\n", len(keys), args[1])
		return nil
	},
}

// --- per-app addon attachments ---

var appsAddonsCmd = &cobra.Command{
	Use:   "addons",
	Short: "Manage which addons inject env vars into a single app",
}

var appsAddonsListCmd = &cobra.Command{
	Use:   "list <project-id> <app-id>",
	Short: "List addons attached to a single app",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		addons, err := client.ListAppAddonAttachments(args[0], args[1])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(addons)
		}
		if len(addons) == 0 {
			fmt.Println("No addons attached. Use `usectl apps addons attach`.")
			return nil
		}
		rows := make([][]string, len(addons))
		for i, a := range addons {
			rows[i] = []string{a.ID, a.AddonType, a.Name, a.Mode, a.Status}
		}
		output.Table([]string{"ID", "TYPE", "NAME", "MODE", "STATUS"}, rows)
		return nil
	},
}

var appsAddonsAttachCmd = &cobra.Command{
	Use:   "attach <project-id> <app-id> <addon-id>",
	Short: "Attach an addon to an app (inject its env vars)",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if err := client.AttachAppAddon(args[0], args[1], args[2]); err != nil {
			return err
		}
		fmt.Println("✓ Addon attached. App pods rolling.")
		return nil
	},
}

var appsAddonsDetachCmd = &cobra.Command{
	Use:   "detach <project-id> <app-id> <addon-id>",
	Short: "Detach an addon from an app (stop injecting its env vars)",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if err := client.DetachAppAddon(args[0], args[1], args[2]); err != nil {
			return err
		}
		fmt.Println("✓ Addon detached. App pods rolling.")
		return nil
	},
}

// --- traffic + insights ---

var appsTrafficCmd = &cobra.Command{
	Use:   "traffic <project-id> <app-id>",
	Short: "Show Traefik request metrics (rate, p50/p95/p99, bytes, codes)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		t, err := client.GetAppTraffic(args[0], args[1])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(t)
		}
		if t.NoRouters {
			fmt.Println("No Traefik routers found for this app yet.")
			return nil
		}
		output.Table([]string{"FIELD", "VALUE"}, [][]string{
			{"Total Requests", strconv.FormatInt(t.RequestsTotal, 10)},
			{"Last 5m Requests", strconv.FormatInt(t.Requests5m, 10)},
			{"Request Rate (req/s)", fmt.Sprintf("%.3f", t.RequestRate)},
			{"Avg Duration", fmt.Sprintf("%.1f ms", t.AvgDurationMs)},
			{"p50 / p95 / p99", fmt.Sprintf("%.1f / %.1f / %.1f ms", t.P50Ms, t.P95Ms, t.P99Ms)},
			{"Bytes In Rate", fmt.Sprintf("%.1f B/s", t.BytesInRate)},
			{"Bytes Out Rate", fmt.Sprintf("%.1f B/s", t.BytesOutRate)},
			{"Open Connections", strconv.FormatInt(t.OpenConnections, 10)},
		})
		if len(t.RequestsByCode) > 0 {
			fmt.Println("\nBy code class:")
			rows := [][]string{}
			for k, v := range t.RequestsByCode {
				rows = append(rows, []string{k, strconv.FormatInt(v, 10)})
			}
			output.Table([]string{"CLASS", "COUNT"}, rows)
		}
		if t.GrafanaURL != "" {
			fmt.Printf("\nGrafana: %s\n", t.GrafanaURL)
		}
		return nil
	},
}

var appsInsightsCmd = &cobra.Command{
	Use:   "insights <project-id> <app-id>",
	Short: "Show per-pod CPU/memory history + recent error logs",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		ins, err := client.GetAppInsights(args[0], args[1])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(ins)
		}
		fmt.Printf("Window: %ds (step %ds)\n\n", ins.WindowSeconds, ins.StepSeconds)
		if len(ins.ResourceHistory) == 0 {
			fmt.Println("No resource history available.")
		} else {
			rows := make([][]string, 0, len(ins.ResourceHistory))
			for _, p := range ins.ResourceHistory {
				lastCPU, lastMem := "-", "-"
				if n := len(p.CPU); n > 0 {
					lastCPU = fmt.Sprintf("%.3f cores", p.CPU[n-1].V)
				}
				if n := len(p.Memory); n > 0 {
					lastMem = fmt.Sprintf("%.0f MiB", p.Memory[n-1].V/(1024*1024))
				}
				rows = append(rows, []string{p.Pod, lastCPU, lastMem,
					strconv.Itoa(len(p.CPU)), strconv.Itoa(len(p.Memory))})
			}
			output.Table([]string{"POD", "CPU (last)", "MEM (last)", "CPU pts", "MEM pts"}, rows)
		}
		if !ins.ErrorsAvailable {
			fmt.Println("\n(error log lookup unavailable)")
			return nil
		}
		fmt.Printf("\nRecent errors (%d):\n", len(ins.RecentErrors))
		for _, e := range ins.RecentErrors {
			fmt.Printf("[%s] %s/%s: %s\n", e.Timestamp, e.Pod, e.Container, e.Line)
		}
		return nil
	},
}

var (
	appResizeMemory   string
	appResizeCPU      string
	appResizeStorage  string
	appResizeStrategy string // mig 054
)

var appsResizeCmd = &cobra.Command{
	Use:   "resize <project-id> <app-id>",
	Short: "Resize a single app's pod (memory / CPU)",
	Long: `Change how much memory and/or CPU each replica of this app gets.

The platform tries an in-place resize first — when the kubelet accepts, live
pods get the new size with no restart and no downtime. If the kubelet rejects
(common reasons: in-place resize not supported by the runtime, working set
above the new memory limit), the deployer falls back to patching the
Deployment template, which triggers a rolling restart.

The namespace ResourceQuota (sized from the project's plan) is the only upper
bound — bigger pods leave less room for other apps and addons in the same
project. A 409 response includes a precise breakdown of where the budget goes.

Use --strategy to switch the Deployment's rollout strategy:
  rolling  (default) — surges a new pod beside the old. Zero downtime when the
                       cluster has room for both pods at once.
  recreate           — kills the old pod, then starts the new. No surge
                       requirement; brief downtime (~10–30s) per deploy.

Examples:
  usectl apps resize <proj> <app> --memory 1Gi --cpu 1 --storage 4Gi
  usectl apps resize <proj> <app> --memory 512Mi
  usectl apps resize <proj> <app> --cpu 500m
  usectl apps resize <proj> <app> --storage 8Gi
  usectl apps resize <proj> <app> --strategy recreate
  usectl apps resize <proj> <app> --memory 2Gi --strategy recreate`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if appResizeMemory == "" && appResizeCPU == "" && appResizeStorage == "" && appResizeStrategy == "" {
			return fmt.Errorf("specify at least one of --memory, --cpu, --storage, or --strategy")
		}
		req := api.ResizeAppRequest{}
		if appResizeMemory != "" {
			mib, err := parseMemoryToMiB(appResizeMemory)
			if err != nil {
				return err
			}
			if mib < 64 {
				return fmt.Errorf("--memory must be at least 64Mi (got %dMi)", mib)
			}
			req.MemoryMiB = &mib
		}
		if appResizeCPU != "" {
			millis, err := parseCPUToMillis(appResizeCPU)
			if err != nil {
				return err
			}
			if millis < 50 {
				return fmt.Errorf("--cpu must be at least 50m (got %dm)", millis)
			}
			req.CPUMillis = &millis
		}
		if appResizeStorage != "" {
			// Storage shares the same human-friendly parser as memory —
			// both are byte quantities; "2Gi", "512Mi", "4G" all valid.
			mib, err := parseMemoryToMiB(appResizeStorage)
			if err != nil {
				return err
			}
			if mib < 256 {
				return fmt.Errorf("--storage must be at least 256Mi (got %dMi)", mib)
			}
			req.StorageMiB = &mib
		}
		if appResizeStrategy != "" {
			if appResizeStrategy != "rolling" && appResizeStrategy != "recreate" {
				return fmt.Errorf("--strategy must be 'rolling' or 'recreate' (got %q)", appResizeStrategy)
			}
			s := appResizeStrategy
			req.RolloutStrategy = &s
		}
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		resp, err := client.ResizeApp(args[0], args[1], req)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(resp)
		}
		mem := "unchanged"
		if resp.App.MemoryMiB != nil {
			mem = fmt.Sprintf("%d MiB", *resp.App.MemoryMiB)
		}
		cpu := "unchanged"
		if resp.App.CPUMillis != nil {
			cpu = fmt.Sprintf("%dm", *resp.App.CPUMillis)
		}
		storage := "platform default"
		if resp.App.StorageMiB != nil {
			storage = fmt.Sprintf("%d MiB", *resp.App.StorageMiB)
		}
		rollout := "rolling (default)"
		if resp.App.RolloutStrategy != nil {
			rollout = *resp.App.RolloutStrategy
		}
		fmt.Printf("App %s resized → memory=%s cpu=%s storage=%s rollout=%s\n", resp.App.Name, mem, cpu, storage, rollout)
		fmt.Printf("Strategy: %s — %s\n", resp.Strategy, resp.Message)
		return nil
	},
}

func init() {
	// create flags
	appsCreateCmd.Flags().StringVar(&appCreateName, "name", "", "App name (required)")
	appsCreateCmd.Flags().StringVar(&appCreateRepo, "repo", "", "GitHub repo URL (required)")
	appsCreateCmd.Flags().StringVar(&appCreateBranch, "branch", "main", "Git branch")
	appsCreateCmd.Flags().StringVar(&appCreateDomain, "domain", "", "Subdomain")
	appsCreateCmd.Flags().IntVar(&appCreatePort, "port", 3000, "Container port")
	appsCreateCmd.Flags().IntVar(&appCreateReplicas, "replicas", 1, "Replica count")
	appsCreateCmd.Flags().Int64Var(&appCreateInstallID, "installation-id", 0, "GitHub App installation ID")
	appsCreateCmd.Flags().BoolVar(&appCreateAutoDeploy, "auto-deploy", false, "Auto-deploy on push")
	appsCreateCmd.Flags().BoolVar(&appCreatePreviewEnvs, "preview-envs", false, "Enable PR preview envs for this app")
	appsCreateCmd.Flags().BoolVar(&appCreatePrivate, "private", false, "Private app (cluster-internal only, no IngressRoute)")
	appsCreateCmd.Flags().StringVar(&appCreateKind, "kind", "web", "Pod kind: web, worker, release")
	appsCreateCmd.Flags().StringVar(&appCreateCommand, "command", "", "Override container command (required for worker/release)")
	appsCreateCmd.Flags().StringSliceVar(&appCreateArgs, "arg", nil, "Container arg (repeatable)")
	appsCreateCmd.MarkFlagRequired("name")
	appsCreateCmd.MarkFlagRequired("repo")

	// update flags
	appsUpdateCmd.Flags().StringVar(&appUpdateBranch, "branch", "", "New branch")
	appsUpdateCmd.Flags().StringVar(&appUpdateDomain, "domain", "", "New subdomain")
	appsUpdateCmd.Flags().IntVar(&appUpdatePort, "port", 0, "New port")
	appsUpdateCmd.Flags().IntVar(&appUpdateReplicas, "replicas", 0, "New replica count")
	appsUpdateCmd.Flags().BoolVar(&appUpdateAutoDep, "auto-deploy", false, "Auto-deploy on push")
	appsUpdateCmd.Flags().BoolVar(&appUpdatePreview, "preview-envs", false, "Enable PR preview envs")
	appsUpdateCmd.Flags().BoolVar(&appUpdatePrivate, "private", false, "Make app private (no IngressRoute)")
	appsUpdateCmd.Flags().StringVar(&appUpdateDotenvP, "dotenv-path", "", `Path to write .env file inside container (e.g. "/var/www/html/.env")`)
	appsUpdateCmd.Flags().BoolVar(&appUpdateDotenvA, "dotenv-auto", false, `Auto-write .env into container working directory`)
	appsUpdateCmd.Flags().StringVar(&appUpdateKind, "kind", "", "Pod kind: web, worker, release")
	appsUpdateCmd.Flags().StringVar(&appUpdateCommand, "command", "", "Override container command")
	appsUpdateCmd.Flags().StringSliceVar(&appUpdateArgs, "args", nil, "Container args")

	// resize flags
	appsResizeCmd.Flags().StringVar(&appResizeMemory, "memory", "", "Per-pod memory (e.g. 1Gi, 512Mi, 768)")
	appsResizeCmd.Flags().StringVar(&appResizeCPU, "cpu", "", "Per-pod CPU (e.g. 1, 0.5, 500m)")
	appsResizeCmd.Flags().StringVar(&appResizeStorage, "storage", "", "Per-pod ephemeral storage (e.g. 4Gi, 2Gi). Default 2 GiB.")
	appsResizeCmd.Flags().StringVar(&appResizeStrategy, "strategy", "", "Rollout strategy: 'rolling' (zero-downtime) or 'recreate' (no surge, brief downtime)")

	// envs subgroup
	appsEnvsCmd.AddCommand(appsEnvsListCmd, appsEnvsSetCmd, appsEnvsDeleteCmd)
	// addons subgroup
	appsAddonsCmd.AddCommand(appsAddonsListCmd, appsAddonsAttachCmd, appsAddonsDetachCmd)

	appsCmd.AddCommand(
		appsListCmd, appsCreateCmd, appsUpdateCmd, appsDeleteCmd,
		appsStartCmd, appsStopCmd, appsRestartCmd, appsResizeCmd,
		appsInternalCmd, appsVariablesCmd, appsRevealCmd,
		appsEnvsCmd, appsAddonsCmd,
		appsTrafficCmd, appsInsightsCmd,
	)
	rootCmd.AddCommand(appsCmd)
}
