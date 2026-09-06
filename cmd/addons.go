package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

var addonsCmd = &cobra.Command{
	Use:     "addons",
	Aliases: []string{"addon"},
	Short:   "Manage machine addons (database, redis, nats, mongodb, s3, ...)",
	Long: `An addon is a packaged service the platform provisions for a machine —
Postgres, Redis, NATS, MongoDB, S3, and so on. Each emits a connection Secret
holding DATABASE_URL, REDIS_URL and friends.

An addon must be ATTACHED to a pod before that pod receives those variables.
Provisioning alone is not enough: a machine can own a database that a given pod
cannot see, and that asymmetry is the usual reason a pod starts but cannot
connect to anything.

  usectl machines pods addons <machine> <pod>          what a pod receives
  usectl machines pods attach-addon <machine> <pod> database

Modes:
  managed    Wired to the shared platform instance. Free, no quota cost, and
             backed up centrally. Not every addon offers anything else.
  dedicated  Its own workload and PVC inside the machine namespace, drawing on
             the machine's CPU/RAM budget. Only addons that publish a size
             ladder support this; the interactive wizard hides the choice for
             the rest rather than offering one the server will reject.

Notes:
  - Never set DATABASE_URL, REDIS_URL and the like by hand. They are injected
    from the addon and would be overwritten.
  - A second instance of a type needs its own --name, and its keys are then
    prefixed with it: --name analytics yields ANALYTICS_DATABASE_URL.
  - Per-addon automated backups apply only to a DEDICATED database; managed
    databases are backed up centrally.
  - 'shareable' read-links an addon owned by another of your machines.
    Unlinking does not deprovision the source.

Subcommands:
  catalog      Addon types available, and the env vars each injects
  list         This machine's addons, with size, version, backups and UI
  get          One addon in full: config, credentials, backups, pods
  add          Provision an addon (interactive when --type is omitted)
  remove       Deprovision an addon
  ui           Toggle the addon's admin UI (pgAdmin, Redis Commander, ...)
  config       Patch behavioural config (e.g. NATS_WS_*)
  start/stop   Scale a dedicated addon to 0 and back`,
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
	Use:   "list <machine>",
	Short: "List a machine's addons with size, version, backups and UI",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID, err := resolveMachine(client, args[0])
		if err != nil {
			return err
		}
		addons, err := client.ListProjectAddons(machineID)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(addons)
		}
		if len(addons) == 0 {
			fmt.Printf("No addons on this machine.\n\nAdd one:\n  usectl machines addons add %s --type database\n", args[0])
			return nil
		}
		// ADDON is "type/name" because a bare name is ambiguous by design —
		// a machine can hold both a "primary" database and a "primary" bucket.
		rows := make([][]string, len(addons))
		for i, a := range addons {
			rows[i] = []string{
				a.AddonType + "/" + a.Name,
				a.Mode,
				a.Status,
				addonSizeCol(a),
				addonVersionCol(a),
				addonBackupCol(a),
				addonUICol(a),
			}
		}
		output.Table([]string{"ADDON", "MODE", "STATUS", "SIZE", "VERSION", "BACKUPS", "UI"}, rows)
		fmt.Printf("\nDetail:\n  usectl machines addons get %s <addon>\n", args[0])
		return nil
	},
}

var (
	addonAddType       string
	addonAddMode       string
	addonAddName       string
	addonAddSharedFrom string
	addonAddReplicas   int
	addonAddSize       string
	addonAddVersion    string
	addonAddRawConfig  string
	addonAddBackup     string
)

var addonsAddCmd = &cobra.Command{
	Use:   "add [machine]",
	Short: "Provision a new addon (managed by default; --mode dedicated for in-namespace)",
	Long: `Provision an addon for a machine. Managed mode wires the machine to a
shared instance (free, no quota cost). Dedicated mode runs the addon
inside the machine namespace with its own PVC and counts against the
plan's CPU/RAM budget.

For dedicated addons you can pick a size preset (small/medium/large/...)
via --size. For multi-instance support (e.g. two databases for one
project) pass --name to label the second instance — env var keys get
prefixed by the upper-cased name (DATABASE_URL → ANALYTICS_DATABASE_URL).`,
	Example: `  # Interactive: pick from the catalogue, then answer the config questions
  usectl machines addons add api

  # Managed Postgres (default mode)
  usectl machines addons add api --type database

  # Dedicated Redis, large preset
  usectl machines addons add api --type redis --mode dedicated --size large

  # Second Postgres instance named "analytics" (keys become ANALYTICS_DATABASE_URL)
  usectl machines addons add api --type database --name analytics

  # Dedicated Postgres with nightly backups
  usectl machines addons add api --type database --mode dedicated \
    --backup-schedule "0 3 * * *"

  # Raw dedicated config
  usectl machines addons add api --type mongodb --mode dedicated \
    --config '{"size":{"name":"medium","cpu_millis":500,"memory_mib":512,"storage_gib":10}}'`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		// Previously the raw argument went straight to the API, so a machine
		// NAME produced "API error 400: invalid project id".
		ref, src, err := machineRef(firstOrEmpty(args))
		if err != nil {
			return err
		}
		echoMachineSource(ref, src)
		machineID, err := resolveMachine(client, ref)
		if err != nil {
			return err
		}

		if interactive() && !cmd.Flags().Changed("type") {
			if err := addonAddWizard(client, machineID); err != nil {
				return err
			}
		}
		if addonAddType == "" {
			return requireInteractive([]string{"type"},
				"usectl machines addons add <machine> --type <database|redis|nats|...> [--mode dedicated]")
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
		if addonAddBackup != "" {
			// Per-addon backup schedule (dedicated database only; the server
			// rejects it elsewhere).
			dedicated["backup_schedule"] = addonAddBackup
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

		addon, err := client.AddProjectAddon(machineID, req)
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
	Use:     "remove [machine] <type-or-id>",
	Aliases: []string{"rm", "delete", "del"},
	Short:   "Remove an addon (by type for the primary instance, or by UUID for any instance)",
	Args:    cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID, addonID, rest := "", "", []string(nil)
		if machineID, addonID, rest, err = resolveMachineAndAddon(client, args); err != nil {
			return err
		}
		args = append([]string{machineID, addonID}, rest...)
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
	Use:   "ui [machine] <type-or-id>",
	Short: "Enable or disable the addon's admin UI (pgAdmin, Redis Commander, ...)",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if addonUIEnable == addonUIDisable {
			return fmt.Errorf("pass exactly one of --enable / --disable")
		}
		enable := addonUIEnable
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID, addonID, rest := "", "", []string(nil)
		if machineID, addonID, rest, err = resolveMachineAndAddon(client, args); err != nil {
			return err
		}
		args = append([]string{machineID, addonID}, rest...)
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
	Use:   "config [machine] <addon-id> <json>",
	Short: "Patch an addon's behavioral config (merged into config JSONB)",
	Long: `Sends a JSON object that is merged into the addon row's config column. Used
for behavioral flags that don't fit other knobs — e.g. NATS_WS_ENABLED.

The merge happens server-side and the addon Provision re-runs so the
cluster reconciles to the new config.`,
	Example: `  usectl machines addons config api nats '{"NATS_WS_ENABLED":"true"}'`,
	Args:    cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID, addonID, rest := "", "", []string(nil)
		if machineID, addonID, rest, err = resolveMachineAndAddon(client, args); err != nil {
			return err
		}
		args = append([]string{machineID, addonID}, rest...)
		// The JSON body is the one argument that cannot be inferred, so it is
		// checked explicitly rather than indexed — the machine and addon may
		// each have come from context, leaving fewer positionals than the
		// pre-resolution arity suggested.
		if len(rest) == 0 {
			return fmt.Errorf("a JSON object is required, e.g. usectl machines addons config %s %s '{\"KEY\":\"value\"}'",
				machineID, addonID)
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
	Use:   "shareable [machine]",
	Short: "List addons from your other projects you can share-link to this one",
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
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
	Use:   "start [machine] <addon-id>",
	Short: "Restore a dedicated addon's replicas (no-op for managed)",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID, addonID, rest := "", "", []string(nil)
		if machineID, addonID, rest, err = resolveMachineAndAddon(client, args); err != nil {
			return err
		}
		args = append([]string{machineID, addonID}, rest...)
		if err := client.StartProjectAddon(args[0], args[1]); err != nil {
			return err
		}
		fmt.Println("✓ Addon starting")
		return nil
	},
}

var addonsStopCmd = &cobra.Command{
	Use:   "stop [machine] <addon-id>",
	Short: "Scale a dedicated addon to 0 (no-op for managed)",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID, addonID, rest := "", "", []string(nil)
		if machineID, addonID, rest, err = resolveMachineAndAddon(client, args); err != nil {
			return err
		}
		args = append([]string{machineID, addonID}, rest...)
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
	addonsAddCmd.Flags().StringVar(&addonAddType, "type", "", "Addon type: database, redis, nats, mongodb, s3, ... (prompted for if omitted on a terminal)")
	addonsAddCmd.Flags().StringVar(&addonAddMode, "mode", "managed", "Mode: managed | dedicated")
	addonsAddCmd.Flags().StringVar(&addonAddName, "name", "", "Instance name (e.g. 'analytics' for a 2nd database). Empty = primary.")
	addonsAddCmd.Flags().StringVar(&addonAddSharedFrom, "shared-from", "", "Source addon UUID to share from (managed mode only)")
	addonsAddCmd.Flags().IntVar(&addonAddReplicas, "replicas", 1, "Replica count (dedicated mode)")
	addonsAddCmd.Flags().StringVar(&addonAddSize, "size", "", "Size preset name for dedicated mode; run the interactive wizard to see the ladder this addon publishes")
	addonsAddCmd.Flags().StringVar(&addonAddVersion, "version", "", "Addon version override (dedicated mode)")
	addonsAddCmd.Flags().StringVar(&addonAddRawConfig, "config", "", "Raw JSON merged into dedicated_config")
	addonsAddCmd.Flags().StringVar(&addonAddBackup, "backup-schedule", "", "Automated backup cron, 5 fields (dedicated database only)")
	addonsAddCmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "Do not prompt; fail instead if a required value is missing")
	// --type is NOT MarkFlagRequired: cobra enforces that before RunE, which
	// would block the interactive catalogue picker. The requirement is checked
	// inside RunE instead, where non-interactive callers still get a clear
	// missing-value error.

	addonsUICmd.Flags().BoolVar(&addonUIEnable, "enable", false, "Enable the addon's admin UI")
	addonsUICmd.Flags().BoolVar(&addonUIDisable, "disable", false, "Disable the addon's admin UI")

	addonsCmd.AddCommand(addonsCatalogCmd, addonsListCmd, addonsAddCmd, addonsRemoveCmd, addonsUICmd,
		addonsConfigCmd, addonsShareableCmd, addonsStartCmd, addonsStopCmd)
	rootCmd.AddCommand(addonsCmd)
}
