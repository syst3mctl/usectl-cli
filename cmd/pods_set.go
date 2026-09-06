package cmd

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

// `usectl machines pods set` — edit one pod's configuration by key=value.
//
// The settable keys span two different endpoints: sizing and rollout strategy
// go to PATCH .../apps/{id}/resources (which can resize live pods in place),
// everything else to PATCH .../apps/{id}. Routing is decided per key here so
// the caller never has to know which is which, and a single invocation
// mixing both still results in at most one call to each.

// podSetting describes one settable key. Keeping this as data rather than a
// switch means `pods set` with no arguments can print the same list it
// accepts, so the command documents itself.
type podSetting struct {
	key      string
	valueDoc string
	help     string
	resource bool // routed to the resize endpoint rather than the update one
}

var podSettings = []podSetting{
	{"port", "<1-65535>", "Primary container port", false},
	{"visibility", "public|internal", "internal removes the IngressRoute; the ClusterIP Service stays", false},
	{"replicas", "<n>", "Replica count", false},
	{"branch", "<name>", "Git branch (repo-sourced pods)", false},
	{"domain", "<host>", "Legacy single-domain field; prefer 'machines domains'", false},
	{"image", "<ref>", "Prebuilt image reference; switches the pod to image source", false},
	{"kind", "web|worker|release", "Pod kind", false},
	{"command", "<string>", "Container entrypoint override", false},
	{"auto-deploy", "true|false", "Deploy automatically on push", false},
	{"metrics", "true|false", "Scrape this pod's /metrics into the platform store", false},
	{"cpu", "<millicores>", "CPU burst limit, e.g. 500m or 500", true},
	{"memory", "<size>", "Memory request+limit, e.g. 512Mi or 1Gi", true},
	{"storage", "<size>", "Ephemeral storage, e.g. 2Gi", true},
	{"rollout", "rolling|recreate", "Rollout strategy; recreate has brief downtime but no surge", true},
}

func podSettingByKey(k string) (podSetting, bool) {
	for _, s := range podSettings {
		if s.key == k {
			return s, true
		}
	}
	return podSetting{}, false
}

var podsSetCmd = &cobra.Command{
	Use:   "set [machine] <pod> [key=value ...]",
	Short: "Change a pod's ports, limits, visibility or rollout strategy",
	Long: `Update one pod's configuration. Run without any key=value pairs to list the
settable keys alongside the pod's current values.

Sizes accept Kubernetes-style suffixes: cpu in millicores (500m), memory and
storage in Mi/Gi (512Mi, 2Gi). A bare number means millicores for cpu and MiB
for memory and storage.

Extra ports are managed with 'pods open-port' / 'pods close-port' rather than
here, because the API replaces the whole extra-port list in one write and a
key=value form would silently drop the ports it did not mention.`,
	Example: `  usectl machines pods set api web                       # show current values
  usectl machines pods set api web port=8080
  usectl machines pods set api web cpu=500m memory=512Mi

  # With a default machine ('usectl use api'), name only the pod:
  usectl machines pods set web rollout=recreate`,
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

		if len(pairs) == 0 {
			return showPodSettings(client, machineID, podID)
		}

		var upd api.UpdateProjectAppRequest
		var res api.ResizeAppRequest
		touchedUpdate, touchedResize := false, false

		for _, pair := range pairs {
			k, v, ok := strings.Cut(pair, "=")
			if !ok {
				return fmt.Errorf("expected key=value, got %q", pair)
			}
			k = strings.ToLower(strings.TrimSpace(k))
			setting, known := podSettingByKey(k)
			if !known {
				return fmt.Errorf("unknown key %q — settable keys: %s", k, strings.Join(podSettingKeys(), ", "))
			}

			switch k {
			case "port":
				n, err := parsePort(v)
				if err != nil {
					return err
				}
				upd.Port = &n
			case "visibility":
				switch v {
				case "public":
					t := true
					upd.IsPublic = &t
				case "internal", "private":
					f := false
					upd.IsPublic = &f
				default:
					return fmt.Errorf("visibility must be 'public' or 'internal', got %q", v)
				}
			case "replicas":
				n, err := strconv.Atoi(v)
				if err != nil || n < 0 {
					return fmt.Errorf("replicas must be a non-negative integer, got %q", v)
				}
				upd.Replicas = &n
			case "branch":
				upd.Branch = &v
			case "domain":
				upd.Domain = &v
			case "image":
				// Switching source is destructive to the repo link server-side,
				// so send source_type explicitly rather than relying on the API
				// to infer it from image_ref alone.
				st := "image"
				upd.SourceType = &st
				upd.ImageRef = &v
			case "kind":
				if v != "web" && v != "worker" && v != "release" {
					return fmt.Errorf("kind must be web, worker or release, got %q", v)
				}
				upd.Kind = &v
			case "command":
				upd.Command = &v
			case "auto-deploy":
				b, err := parseBool(k, v)
				if err != nil {
					return err
				}
				upd.AutoDeploy = &b
			case "metrics":
				b, err := parseBool(k, v)
				if err != nil {
					return err
				}
				upd.MetricsEnabled = &b
			case "cpu":
				n, err := parseMillicores(v)
				if err != nil {
					return err
				}
				res.CPUMillis = &n
			case "memory":
				n, err := parseMiB(v)
				if err != nil {
					return err
				}
				res.MemoryMiB = &n
			case "storage":
				n, err := parseMiB(v)
				if err != nil {
					return err
				}
				res.StorageMiB = &n
			case "rollout":
				if v != "rolling" && v != "recreate" {
					return fmt.Errorf("rollout must be 'rolling' or 'recreate', got %q", v)
				}
				res.RolloutStrategy = &v
			}

			if setting.resource {
				touchedResize = true
			} else {
				touchedUpdate = true
			}
		}

		if touchedUpdate {
			_, warning, err := client.UpdateProjectApp(machineID, podID, upd)
			if err != nil {
				return err
			}
			if warning != "" {
				fmt.Printf("⚠ %s\n", warning)
			}
		}
		if touchedResize {
			resp, err := client.ResizeApp(machineID, podID, res)
			if err != nil {
				return err
			}
			// The strategy is worth surfacing: "in_place" means live pods took
			// the new size without restarting, "rolling_restart" means they
			// did not — a materially different outcome for a running service.
			if resp != nil && resp.Strategy != "" {
				fmt.Printf("  resize: %s", resp.Strategy)
				if resp.Message != "" {
					fmt.Printf(" — %s", resp.Message)
				}
				fmt.Println()
			}
		}
		fmt.Println("✓ Pod updated.")
		return nil
	},
}

func podSettingKeys() []string {
	keys := make([]string, len(podSettings))
	for i, s := range podSettings {
		keys[i] = s.key
	}
	sort.Strings(keys)
	return keys
}

// showPodSettings prints every settable key with the pod's current value, so
// `set` with no pairs answers "what can I change and what is it now?".
func showPodSettings(client *api.Client, machineID, podID string) error {
	apps, err := client.ListProjectApps(machineID)
	if err != nil {
		return err
	}
	var a *api.ProjectApp
	for i := range apps {
		if apps[i].ID == podID {
			a = &apps[i]
			break
		}
	}
	if a == nil {
		return fmt.Errorf("pod %s not found", podID)
	}

	cur := map[string]string{
		"port":        strconv.Itoa(a.Port),
		"visibility":  map[bool]string{true: "public", false: "internal"}[a.IsPublic],
		"replicas":    strconv.Itoa(a.Replicas),
		"branch":      orDash(a.Branch),
		"domain":      orDash(a.Domain),
		"image":       orDash(a.ImageRef),
		"kind":        orDash(a.Kind),
		"command":     orDashPtr(a.Command),
		"auto-deploy": strconv.FormatBool(a.AutoDeploy),
		"metrics":     strconv.FormatBool(a.MetricsEnabled),
		"cpu":         intPtrOr(a.CPUMillis, "250 (default)", "m"),
		"memory":      intPtrOr(a.MemoryMiB, "256 (default)", "Mi"),
		"storage":     intPtrOr(a.StorageMiB, "2048 (default)", "Mi"),
		"rollout":     strPtrOr(a.RolloutStrategy, "rolling (default)"),
	}

	if jsonOutput {
		return output.JSON(cur)
	}
	rows := make([][]string, len(podSettings))
	for i, s := range podSettings {
		rows[i] = []string{s.key, cur[s.key], s.valueDoc, s.help}
	}
	output.Table([]string{"KEY", "CURRENT", "ACCEPTS", "DESCRIPTION"}, rows)
	fmt.Println("\nExtra ports are managed separately:")
	fmt.Println("  usectl machines pods open-port <machine> <pod> <port>[/proto] [name]")
	fmt.Println("  usectl machines pods close-port <machine> <pod> <port>")
	return nil
}

// ---- value parsers ---------------------------------------------------------

func parsePort(v string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 1 || n > 65535 {
		return 0, fmt.Errorf("port must be 1-65535, got %q", v)
	}
	return n, nil
}

func parseBool(k, v string) (bool, error) {
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		return false, fmt.Errorf("%s must be true or false, got %q", k, v)
	}
	return b, nil
}

// parseMillicores accepts "500m" or "500"; a bare number is already millicores.
// A fractional core ("0.5") is accepted too, since that is how CPU is usually
// discussed even though the API takes millicores.
func parseMillicores(v string) (int, error) {
	s := strings.TrimSpace(strings.ToLower(v))
	if strings.HasSuffix(s, "m") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "m"))
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("cpu must be a positive number of millicores, got %q", v)
		}
		return n, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 {
		return 0, fmt.Errorf("cpu must be a positive number, got %q", v)
	}
	if f < 16 {
		// Values this small are cores, not millicores: nobody asks for 2
		// millicores, and 2 cores is a normal request.
		return int(f * 1000), nil
	}
	return int(f), nil
}

// parseMiB accepts "512Mi", "1Gi", "512M", "1G" or a bare number of MiB.
func parseMiB(v string) (int, error) {
	s := strings.TrimSpace(strings.ToLower(v))
	mult := 1.0
	switch {
	case strings.HasSuffix(s, "gi"), strings.HasSuffix(s, "gb"):
		s, mult = s[:len(s)-2], 1024
	case strings.HasSuffix(s, "mi"), strings.HasSuffix(s, "mb"):
		s = s[:len(s)-2]
	case strings.HasSuffix(s, "g"):
		s, mult = strings.TrimSuffix(s, "g"), 1024
	case strings.HasSuffix(s, "m"):
		s = strings.TrimSuffix(s, "m")
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || f <= 0 {
		return 0, fmt.Errorf("%q is not a size — use e.g. 512Mi, 2Gi, or a number of MiB", v)
	}
	return int(f * mult), nil
}

func orDashPtr(s *string) string {
	if s == nil || *s == "" {
		return "—"
	}
	return *s
}

func intPtrOr(p *int, def, unit string) string {
	if p == nil {
		return def
	}
	return strconv.Itoa(*p) + unit
}

func strPtrOr(p *string, def string) string {
	if p == nil || *p == "" {
		return def
	}
	return *p
}
