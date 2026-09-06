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

// `usectl machines settings` — machine-level configuration.
//
// Only genuinely machine-wide knobs live here. UpdateProjectRequest still
// accepts branch, domain and port, but those are per-POD settings on any
// machine created since mig 013 and are deliberately NOT exposed: offering
// them would invite someone to set a "machine branch" that no pod reads.
// They belong to `machines pods set`.

type machineSetting struct {
	key      string
	valueDoc string
	help     string
}

var machineSettings = []machineSetting{
	{"name", "<name>", "Machine name. Changes the derived namespace — see the warning below"},
	{"preview-envs", "true|false", "Create a preview deployment for each pull request"},
	{"backup-schedule", "<cron>|off", "Legacy machine-wide DB backup; prefer per-addon backups"},
	{"installation-id", "<id>", "GitHub App installation used to clone private repos"},
}

var machineSettingsCmd = &cobra.Command{
	Use:   "settings [machine] [key=value ...]",
	Short: "Show or change machine-level settings",
	Long: `Run without key=value pairs to list the settable keys and their current
values.

Repository, branch, domain, port and container sizing are NOT here — they are
per-pod settings. Use 'usectl machines pods set' for those, and
'usectl machines quota' to change vCPU/RAM/storage.`,
	Example: `  usectl machines settings api
  usectl machines settings api preview-envs=true
  usectl machines settings api name=api-v2`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		ref, src, err := machineRef(firstOrEmpty(args))
		if err != nil {
			return err
		}
		echoMachineSource(ref, src)
		machineID, err := resolveMachine(client, ref)
		if err != nil {
			return err
		}
		resp, err := client.GetProjectFull(machineID)
		if err != nil {
			return err
		}
		p := resp.Project

		if len(args) == 1 {
			cur := map[string]string{
				"name":            p.Name,
				"preview-envs":    strconv.FormatBool(p.EnablePreviewEnvs),
				"backup-schedule": strPtrOr(p.BackupSchedule, "off"),
				"installation-id": int64PtrOr(p.InstallationID, "—"),
			}
			if jsonOutput {
				return output.JSON(cur)
			}
			rows := make([][]string, len(machineSettings))
			for i, s := range machineSettings {
				rows[i] = []string{s.key, cur[s.key], s.valueDoc, s.help}
			}
			output.Table([]string{"KEY", "CURRENT", "ACCEPTS", "DESCRIPTION"}, rows)
			fmt.Println("\nElsewhere:")
			fmt.Printf("  sizing / billing   usectl machines quota %s\n", args[0])
			fmt.Printf("  per-pod config     usectl machines pods set %s <pod>\n", args[0])
			fmt.Printf("  addon config       usectl machines addons get %s <addon>\n", args[0])
			return nil
		}

		var req api.UpdateProjectRequest
		renaming := ""
		for _, pair := range args[1:] {
			k, v, ok := strings.Cut(pair, "=")
			if !ok {
				return fmt.Errorf("expected key=value, got %q", pair)
			}
			switch strings.ToLower(strings.TrimSpace(k)) {
			case "name":
				renaming = v
				req.Name = &v
			case "preview-envs":
				b, err := parseBool("preview-envs", v)
				if err != nil {
					return err
				}
				req.EnablePreviewEnvs = &b
			case "backup-schedule":
				if v == "off" {
					v = ""
				}
				return fmt.Errorf("machine-wide backup schedules are legacy — set it on the database addon instead:\n  usectl machines addons get %s database", args[0])
			case "installation-id":
				n, err := strconv.ParseInt(v, 10, 64)
				if err != nil {
					return fmt.Errorf("installation-id must be a number, got %q", v)
				}
				req.InstallationID = &n
			default:
				return fmt.Errorf("unknown key %q — settable keys: %s", k, strings.Join(machineSettingKeys(), ", "))
			}
		}

		// A rename changes the derived namespace (kdeploy-<name>), so the next
		// reconcile builds the machine's resources somewhere new and leaves the
		// old namespace behind. That is not obvious from "set a name", so it is
		// confirmed explicitly rather than mentioned afterwards.
		if renaming != "" {
			fmt.Printf("Renaming %s → %s\n", p.Name, renaming)
			fmt.Printf("  The namespace is derived from the name: kdeploy-%s → kdeploy-%s.\n", p.Name, renaming)
			fmt.Println("  Existing pods, addons and PVCs stay in the OLD namespace until they are")
			fmt.Println("  redeployed, and are not migrated automatically.")
			if !confirm("Continue?") {
				return fmt.Errorf("cancelled")
			}
		}

		if _, err := client.UpdateProject(machineID, req); err != nil {
			return err
		}
		fmt.Println("✓ Settings updated.")
		return nil
	},
}

func int64PtrOr(p *int64, def string) string {
	if p == nil || *p == 0 {
		return def
	}
	return strconv.FormatInt(*p, 10)
}

func machineSettingKeys() []string {
	keys := make([]string, len(machineSettings))
	for i, s := range machineSettings {
		keys[i] = s.key
	}
	sort.Strings(keys)
	return keys
}

func init() {
	projectsCmd.AddCommand(machineSettingsCmd, machineUsageCmd)
}
