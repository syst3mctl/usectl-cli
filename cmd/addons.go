package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

var addonsCmd = &cobra.Command{
	Use:     "addons",
	Aliases: []string{"addon"},
	Short:   "Manage machine addons (database, redis, nats, mongodb, s3, ...)",
	Long: `An addon is a packaged service the platform provisions and wires into your
project's namespace. Each addon emits a connection Secret that gets injected
into the machine's pods as env vars (DATABASE_URL, REDIS_URL, etc.).

Agent tips:
  - Addon credentials are injected automatically. Do NOT set DATABASE_URL,
    REDIS_URL, etc. via 'usectl envs set' — they will be overridden and
    secrets may leak into the env vault.
  - 'usectl addons catalog' lists every addon type and the exact env vars
    it injects. Read this before deciding which addon a project needs.
  - Addons live on the machine, not on individual apps. Every app pod in
    the machine sees the same DATABASE_URL by default. Use 'usectl apps
    addons' to scope an addon to a specific app instead.
  - 'shareable' lets one machine read-link an addon owned by another of
    your machines — useful for connecting a worker machine to the main
    machine's DB without re-provisioning. Unlinking does not deprovision
    the source.

Modes:
  managed    Shared instance — free, doesn't count against project budget.
  dedicated  Workload runs inside the machine namespace with its own PVC.

Subcommands:
  catalog          List addons available in the catalog
  list             List addons enabled on a project
  add              Provision a new addon
  remove           Remove an addon
  ui               Toggle the addon's admin UI (pgAdmin, Redis Commander, ...)
  config           Update an addon's behavioral config (e.g. NATS_WS_*)
  shareable        List addons from your other projects you can share-link
  start/stop       Scale a dedicated addon to 0 / restore replicas`,
}

var addonsCatalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "List all addons available in the catalog",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		entries, err := client.AddonCatalog()
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(entries)
		}
		rows := make([][]string, len(entries))
		for i, e := range entries {
			ui := "-"
			if e.HasUI {
				ui = e.UITool
				if ui == "" {
					ui = "yes"
				}
			}
			rows[i] = []string{e.Type, e.Name, ui, strings.Join(e.EnvVars, ",")}
		}
		output.Table([]string{"TYPE", "NAME", "UI", "ENV VARS"}, rows)
		return nil
	},
}

var addonsListCmd = &cobra.Command{
	Use:   "list <project-id>",
	Short: "List addons enabled on a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		addons, err := client.ListProjectAddons(args[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(addons)
		}
		if len(addons) == 0 {
			fmt.Println("No addons enabled. Use `usectl addons add`.")
			return nil
		}
		rows := make([][]string, len(addons))
		for i, a := range addons {
			rows[i] = []string{a.ID, a.AddonType, a.Name, a.Mode, a.Status, strconv.FormatBool(a.UIEnabled)}
		}
		output.Table([]string{"ID", "TYPE", "NAME", "MODE", "STATUS", "UI"}, rows)
		return nil
	},
}

var (
	addonAddType        string
	addonAddMode        string
	addonAddName        string
	addonAddSharedFrom  string
	addonAddReplicas    int
	addonAddSize        string
	addonAddVersion     string
	addonAddRawConfig   string
)

var addonsAddCmd = &cobra.Command{
	Use:   "add <project-id>",
	Short: "Provision a new addon (managed by default; --mode dedicated for in-namespace)",
	Long: `Provision an addon for a machine. Managed mode wires the machine to a
shared instance (free, no quota cost). Dedicated mode runs the addon
inside the machine namespace with its own PVC and counts against the
plan's CPU/RAM budget.

For dedicated addons you can pick a size preset (small/medium/large/...)
via --size. For multi-instance support (e.g. two databases for one
project) pass --name to label the second instance — env var keys get
prefixed by the upper-cased name (DATABASE_URL → ANALYTICS_DATABASE_URL).`,
	Example: `  # Managed Postgres (default)
  usectl addons add proj-id --type database

  # Dedicated Redis, large preset
  usectl addons add proj-id --type redis --mode dedicated --size large

  # Second Postgres instance, named "analytics"
  usectl addons add proj-id --type database --name analytics

  # Pass arbitrary dedicated config
  usectl addons add proj-id --type mongodb --mode dedicated \
    --config '{"size":{"name":"medium","cpu_millis":500,"memory_mib":512,"storage_gib":10}}'`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}

		req := api.AddProjectAddonRequest{
			AddonType: addonAddType,
			Mode:      addonAddMode,
			Name:      addonAddName,
			Replicas:  addonAddReplicas,
		}
		if addonAddSharedFrom != "" {
			req.SharedFrom = &addonAddSharedFrom
		}

		// Build dedicated_config from --size/--version/--config flags.
		dedicated := map[string]interface{}{}
		if addonAddSize != "" {
			dedicated["size"] = map[string]interface{}{"name": addonAddSize}
		}
		if addonAddVersion != "" {
			dedicated["version"] = addonAddVersion
		}
		if addonAddRawConfig != "" {
			var raw map[string]interface{}
			if err := json.Unmarshal([]byte(addonAddRawConfig), &raw); err != nil {
				return fmt.Errorf("invalid --config JSON: %w", err)
			}
			for k, v := range raw {
				dedicated[k] = v
			}
		}
		if len(dedicated) > 0 {
			b, _ := json.Marshal(dedicated)
			req.DedicatedConfig = b
		}

		addon, err := client.AddProjectAddon(args[0], req)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(addon)
		}
		fmt.Printf("✓ Addon %s (%s, %s) status=%s\n", addon.AddonType, addon.Name, addon.Mode, addon.Status)
		if addon.Status == "provisioning" {
			fmt.Println("  Provisioning in the background; check `usectl addons list` for status.")
		}
		return nil
	},
}

var addonsRemoveCmd = &cobra.Command{
	Use:   "remove <project-id> <type-or-id>",
	Aliases: []string{"rm"},
	Short: "Remove an addon (by type for the primary instance, or by UUID for any instance)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		// Heuristic: if it looks like a UUID, hit by-id. Otherwise treat as type.
		ident := args[1]
		var rerr error
		if looksLikeUUID(ident) {
			rerr = client.RemoveProjectAddonByID(args[0], ident)
		} else {
			rerr = client.RemoveProjectAddon(args[0], ident)
		}
		if rerr != nil {
			return rerr
		}
		fmt.Println("✓ Addon removed")
		return nil
	},
}

var (
	addonUIEnable  bool
	addonUIDisable bool
)

var addonsUICmd = &cobra.Command{
	Use:   "ui <project-id> <type-or-id>",
	Short: "Enable or disable the addon's admin UI (pgAdmin, Redis Commander, ...)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if addonUIEnable == addonUIDisable {
			return fmt.Errorf("pass exactly one of --enable / --disable")
		}
		enable := addonUIEnable
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		ident := args[1]
		var rerr error
		if looksLikeUUID(ident) {
			rerr = client.ToggleAddonUIByID(args[0], ident, enable)
		} else {
			rerr = client.ToggleAddonUI(args[0], ident, enable)
		}
		if rerr != nil {
			return rerr
		}
		state := "disabled"
		if enable {
			state = "enabled"
		}
		fmt.Printf("✓ Addon UI %s\n", state)
		return nil
	},
}

var addonsConfigCmd = &cobra.Command{
	Use:   "config <project-id> <addon-id> <json>",
	Short: "Patch an addon's behavioral config (merged into config JSONB)",
	Long: `Sends a JSON object that is merged into the addon row's config column. Used
for behavioral flags that don't fit other knobs — e.g. NATS_WS_ENABLED.

The merge happens server-side and the addon Provision re-runs so the
cluster reconciles to the new config.`,
	Example: `  usectl addons config proj-id 4f1c... '{"NATS_WS_ENABLED":"true"}'`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(args[2]), &raw); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
		if err := client.UpdateAddonConfig(args[0], args[1], raw); err != nil {
			return err
		}
		fmt.Println("✓ Addon config updated")
		return nil
	},
}

var addonsShareableCmd = &cobra.Command{
	Use:   "shareable <project-id>",
	Short: "List addons from your other projects you can share-link to this one",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		addons, err := client.ListShareableAddons(args[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(addons)
		}
		if len(addons) == 0 {
			fmt.Println("No shareable addons available.")
			return nil
		}
		rows := make([][]string, len(addons))
		for i, a := range addons {
			rows[i] = []string{a.ID, a.ProjectID, a.AddonType, a.Name, a.Mode, a.Status}
		}
		output.Table([]string{"ID", "SOURCE PROJECT", "TYPE", "NAME", "MODE", "STATUS"}, rows)
		return nil
	},
}

var addonsStartCmd = &cobra.Command{
	Use:   "start <project-id> <addon-id>",
	Short: "Restore a dedicated addon's replicas (no-op for managed)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if err := client.StartProjectAddon(args[0], args[1]); err != nil {
			return err
		}
		fmt.Println("✓ Addon starting")
		return nil
	},
}

var addonsStopCmd = &cobra.Command{
	Use:   "stop <project-id> <addon-id>",
	Short: "Scale a dedicated addon to 0 (no-op for managed)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if err := client.StopProjectAddon(args[0], args[1]); err != nil {
			return err
		}
		fmt.Println("✓ Addon stopped")
		return nil
	},
}

// looksLikeUUID returns true for canonical 36-char UUIDs. Used to disambiguate
// `usectl addons remove <proj> <ident>` between addon-type and addon-id.
func looksLikeUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if r != '-' {
				return false
			}
			continue
		}
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

func init() {
	addonsAddCmd.Flags().StringVar(&addonAddType, "type", "", "Addon type: database, redis, nats, mongodb, s3, ... (required)")
	addonsAddCmd.Flags().StringVar(&addonAddMode, "mode", "managed", "Mode: managed | dedicated")
	addonsAddCmd.Flags().StringVar(&addonAddName, "name", "", "Instance name (e.g. 'analytics' for a 2nd database). Empty = primary.")
	addonsAddCmd.Flags().StringVar(&addonAddSharedFrom, "shared-from", "", "Source addon UUID to share from (managed mode only)")
	addonsAddCmd.Flags().IntVar(&addonAddReplicas, "replicas", 1, "Replica count (dedicated mode)")
	addonsAddCmd.Flags().StringVar(&addonAddSize, "size", "", "Size preset (dedicated mode): small | medium | large | ...")
	addonsAddCmd.Flags().StringVar(&addonAddVersion, "version", "", "Addon version override (dedicated mode)")
	addonsAddCmd.Flags().StringVar(&addonAddRawConfig, "config", "", "Raw JSON merged into dedicated_config")
	addonsAddCmd.MarkFlagRequired("type")

	addonsUICmd.Flags().BoolVar(&addonUIEnable, "enable", false, "Enable the addon's admin UI")
	addonsUICmd.Flags().BoolVar(&addonUIDisable, "disable", false, "Disable the addon's admin UI")

	addonsCmd.AddCommand(addonsCatalogCmd, addonsListCmd, addonsAddCmd, addonsRemoveCmd, addonsUICmd,
		addonsConfigCmd, addonsShareableCmd, addonsStartCmd, addonsStopCmd)
	rootCmd.AddCommand(addonsCmd)
}
