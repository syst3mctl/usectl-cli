package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

// `usectl machines addons get` — the addon equivalent of `machines pods`.
//
// An addon's `config` map mixes three unrelated things, and showing them as
// one flat blob (which is all the CLI could do before) is unreadable:
//
//	DATABASE_URL, S3_BUCKET, ...   env vars actually injected into attached pods
//	_DBUI_URL/_USER/_PASS          credentials for the addon's web UI
//	_db_user, _db_pass, ...        internal metadata the platform keeps
//
// They are separated here. `dedicated_config` carries the tunables the UI
// exposes — engine version, size, storage, connection limits, extensions and
// the automated-backup schedule.
//
// Secret values are MASKED by default. Several of these are live credentials
// (DATABASE_URL embeds the password), and the default rendering of a read-only
// inspection command should not put them into terminal scrollback or CI logs.
// --reveal opts in.

var (
	addonGetReveal bool
)

// secretKey reports whether a config value must be masked unless the caller
// explicitly asks to see it.
//
// Keyed on the name for credentials, but on the VALUE for URLs: a DSN like
// postgres://user:password@host/db must be masked, while a bare endpoint like
// http://external-s3.kdeploy-system.svc:8333 carries nothing secret and is
// more useful shown. Masking on the name alone hid AWS_ENDPOINT_URL while
// leaving the identical S3_ENDPOINT in the clear.
func secretKey(k, v string) bool {
	u := strings.ToUpper(k)
	for _, marker := range []string{"PASS", "SECRET", "TOKEN", "_KEY", "KEY_ID"} {
		if strings.Contains(u, marker) {
			return true
		}
	}
	return urlHasCredentials(v)
}

// urlHasCredentials reports whether a URL carries a userinfo section, i.e.
// scheme://user:pass@host.
func urlHasCredentials(v string) bool {
	i := strings.Index(v, "://")
	if i < 0 {
		return false
	}
	rest := v[i+3:]
	at := strings.Index(rest, "@")
	if at < 0 {
		return false
	}
	// A "@" after the first "/" is part of the path, not userinfo.
	if slash := strings.Index(rest, "/"); slash >= 0 && slash < at {
		return false
	}
	return true
}

func maskValue(v string, reveal bool) string {
	if reveal || v == "" {
		return v
	}
	if len(v) <= 8 {
		return strings.Repeat("•", len(v))
	}
	return v[:4] + strings.Repeat("•", 8) + v[len(v)-2:]
}

// classifyConfig splits the flat config map into the three groups above.
//
// envPrefix must be the addon's own prefix ("PAINBOARD_" for a second
// instance, empty for "primary"). A non-primary instance has that prefix
// stamped onto EVERY key, so the internal marker moves from a leading "_" to
// an embedded "__": _db_pass becomes PAINBOARD__db_pass. Classifying on the
// raw key alone therefore reports platform metadata — including the raw
// password — as a variable the pod receives, which it is not.
func classifyConfig(cfg map[string]string, envPrefix string) (env, ui, internal []string) {
	for k := range cfg {
		bare := k
		if envPrefix != "" {
			bare = strings.TrimPrefix(k, envPrefix)
		}
		switch {
		case strings.HasPrefix(bare, "_DBUI_") || strings.HasPrefix(bare, "_UI_"):
			ui = append(ui, k)
		case strings.HasPrefix(bare, "_"):
			internal = append(internal, k)
		default:
			env = append(env, k)
		}
	}
	sort.Strings(env)
	sort.Strings(ui)
	sort.Strings(internal)
	return
}

var addonsGetCmd = &cobra.Command{
	Use:     "get [machine] <addon>",
	Aliases: []string{"show", "info"},
	Short:   "Show an addon's configuration, credentials, backups and pods",
	Long: `Full detail for one addon: mode and status, the environment variables it
injects into attached pods, its web-UI credentials, engine configuration
(version, size, storage, connection limits, extensions), the automated-backup
schedule, its running pods, and which pods it is attached to.

Secret values are masked unless --reveal is passed. DATABASE_URL and the
S3 keys are live credentials, so they are not printed by default.

The addon may be named by its instance name, its type, or "type/name".`,
	Example: `  usectl machines addons get api database
  usectl machines addons get api database/analytics --reveal
  usectl machines addons get api database --json`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		// Shared helper so machine/addon order is interchangeable here too;
		// this command predates it and resolved positionally.
		machineID, addonID, _, err := resolveMachineAndAddon(client, args)
		if err != nil {
			return err
		}
		addons, err := client.ListProjectAddons(machineID)
		if err != nil {
			return err
		}
		var a *api.ProjectAddon
		for i := range addons {
			if addons[i].ID == addonID {
				a = &addons[i]
				break
			}
		}
		if a == nil {
			return fmt.Errorf("addon %s not found in this machine", addonID)
		}

		if jsonOutput {
			// The JSON path returns the addon verbatim, secrets included: a
			// caller piping to jq has asked for the data, and masking would
			// make the output useless for automation.
			return output.JSON(a)
		}

		pods, _ := client.ListNamespacePods(machineID)
		v := &podsView{addons: addons, pods: pods}
		instances := v.addonInstances(*a, map[string]bool{})

		// Which pods receive this addon's env vars.
		var attachedTo []string
		if apps, aErr := client.ListProjectApps(machineID); aErr == nil {
			for _, app := range apps {
				att, e := client.ListAppAddonAttachments(machineID, app.ID)
				if e != nil {
					continue
				}
				for _, x := range att {
					if x.ID == a.ID {
						attachedTo = append(attachedTo, app.Name)
					}
				}
			}
		}

		printAddonDetail(a, instances, attachedTo, addonGetReveal)
		return nil
	},
}

func printAddonDetail(a *api.ProjectAddon, instances []api.NamespacePod, attachedTo []string, reveal bool) {
	fmt.Printf("%s  %s  %s\n", output.Bold(a.AddonType+"/"+a.Name),
		output.Cyan("["+a.Mode+"]"), addonStatusColored(a.Status))
	fmt.Printf("  %-14s %s\n", "id", a.ID)
	if a.WorkloadName != "" {
		fmt.Printf("  %-14s %s\n", "workload", a.WorkloadName)
	}
	if a.SharedFrom != nil && *a.SharedFrom != "" {
		fmt.Printf("  %-14s %s\n", "shared from", *a.SharedFrom)
	}
	if a.Mode == "dedicated" {
		state := "running"
		if a.IsStopped {
			state = "stopped (scaled to 0; PVC preserved)"
		}
		fmt.Printf("  %-14s %d replica(s), %s\n", "scale", a.Replicas, state)
	}
	if len(attachedTo) > 0 {
		sort.Strings(attachedTo)
		fmt.Printf("  %-14s %s\n", "attached to", strings.Join(attachedTo, ", "))
	} else {
		fmt.Printf("  %s %s\n", output.Dim(output.Pad("attached to", 14)), output.Yellow("no pods — nothing receives these variables yet"))
	}

	env, ui, internal := classifyConfig(a.Config, addonEnvPrefix(a))

	if len(env) > 0 {
		fmt.Println("\n  " + output.Bold("ENVIRONMENT VARIABLES") + output.Dim(" (injected into attached pods)"))
		for _, k := range env {
			val := a.Config[k]
			if secretKey(k, val) {
				val = output.Dim(maskValue(val, reveal))
			}
			fmt.Printf("    %s %s\n", output.Pad(output.Cyan(k), 28), val)
		}
	}

	if len(ui) > 0 {
		label := "WEB UI"
		if !a.UIEnabled {
			label = "WEB UI (disabled)"
		}
		fmt.Printf("\n  %s\n", label)
		for _, k := range ui {
			val := a.Config[k]
			if secretKey(k, val) {
				val = maskValue(val, reveal)
			}
			fmt.Printf("    %-28s %s\n", strings.TrimPrefix(k, "_"), val)
		}
	}

	printDedicatedConfig(a.DedicatedConfig, a)

	if len(internal) > 0 && reveal {
		// Platform bookkeeping — only shown on explicit --reveal, since it is
		// noise for everyone who is not debugging the provisioner.
		fmt.Println("\n  INTERNAL")
		for _, k := range internal {
			fmt.Printf("    %-28s %s\n", k, a.Config[k])
		}
	}

	if len(instances) > 0 {
		fmt.Println("\n  " + output.Bold("PODS"))
		for _, p := range instances {
			fmt.Printf("    %s %s %s  %s\n",
				output.Pad(p.Name, 42), output.Pad(phaseColored(p), 12),
				restartsColored(p.Restarts), output.Dim(orDash(p.NodeName)))
		}
	}
	if !reveal {
		fmt.Println("\n  " + output.Dim("(secrets masked — pass --reveal to show)"))
	}
}

// printDedicatedConfig renders the engine tunables the dashboard exposes.
// addonEnvPrefix returns the prefix stamped on this instance's keys. The API
// leaves env_prefix empty on some rows, so it is derived from the instance
// name as a fallback — matching how the provisioner builds it.
// addonStatusColored: provisioned is the only healthy resting state; failed is
// the only terminal one. Everything else is transient and reads as amber.
func addonStatusColored(status string) string {
	switch status {
	case "provisioned":
		return output.Green(status)
	case "failed":
		return output.Red(status)
	case "stopped":
		return output.Dim(status)
	default:
		return output.Yellow(status)
	}
}

func addonEnvPrefix(a *api.ProjectAddon) string {
	if a.EnvPrefix != "" {
		return a.EnvPrefix
	}
	if a.Name == "" || a.Name == "primary" {
		return ""
	}
	return strings.ToUpper(a.Name) + "_"
}

func printDedicatedConfig(raw json.RawMessage, a *api.ProjectAddon) {
	var m map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	// A managed database has an empty dedicated_config but still has a
	// meaningful backup story ("handled centrally"), so it must not be
	// short-circuited here — that is exactly the question this section is
	// meant to answer.
	isDB := a.AddonType == "database"
	if len(m) == 0 && !isDB {
		return
	}

	fmt.Println("\n  " + output.Bold("CONFIGURATION"))

	if v, ok := m["version"].(string); ok && v != "" {
		fmt.Printf("    %-28s %s\n", "version", v)
	}
	if sz, ok := m["size"].(map[string]any); ok {
		fmt.Printf("    %-28s %s\n", "size", sizeSummary(sz))
	}
	// storage_gb and size.storage_gib are separate fields that can disagree
	// (a resize writes one but not the other). Print both rather than pick a
	// winner and quietly show a number the volume does not have.
	if g, ok := numField(m, "storage_gb"); ok {
		label := "storage"
		if sz, ok2 := m["size"].(map[string]any); ok2 {
			if sg, ok3 := numField(sz, "storage_gib"); ok3 && sg != g {
				fmt.Printf("    %-28s %gGB (size preset says %gGiB — they disagree)\n", label, g, sg)
				goto rest
			}
		}
		fmt.Printf("    %-28s %gGB\n", label, g)
	}
rest:
	if v, ok := numField(m, "max_connections"); ok {
		fmt.Printf("    %-28s %g\n", "max_connections", v)
	}
	if v, ok := numField(m, "shared_buffers_mb"); ok {
		fmt.Printf("    %-28s %gMB\n", "shared_buffers", v)
	}
	if ext, ok := m["extensions"].([]any); ok {
		if len(ext) == 0 {
			fmt.Printf("    %-28s none\n", "extensions")
		} else {
			parts := make([]string, len(ext))
			for i, e := range ext {
				parts[i] = fmt.Sprint(e)
			}
			fmt.Printf("    %-28s %s\n", "extensions", strings.Join(parts, ", "))
		}
	}

	// Automated backups. Absent or empty means disabled — worth stating
	// outright, since "no backup line" reads as "not applicable".
	sched, _ := m["backup_schedule"].(string)
	sched = strings.TrimSpace(sched)
	switch {
	case a.AddonType != "database":
		// No backup concept for this addon type; saying "disabled" would
		// imply one exists.
	case a.Mode != "dedicated":
		fmt.Printf("    %-28s handled centrally by the platform\n", "automated backups")
	case sched == "":
		fmt.Printf("    %-28s disabled\n", "automated backups")
	default:
		fmt.Printf("    %-28s %s  (retention %dd)\n", "automated backups", sched, retentionForSchedule(sched))
	}

	// Anything the platform added that this renderer does not know about is
	// still shown, so a new tunable is never silently invisible.
	known := map[string]bool{"version": true, "size": true, "storage_gb": true,
		"max_connections": true, "shared_buffers_mb": true, "extensions": true,
		"backup_schedule": true}
	var extra []string
	for k := range m {
		if !known[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	for _, k := range extra {
		b, _ := json.Marshal(m[k])
		fmt.Printf("    %-28s %s\n", k, string(b))
	}
}

func sizeSummary(sz map[string]any) string {
	name, _ := sz["name"].(string)
	cpu, _ := numField(sz, "cpu_millis")
	mem, _ := numField(sz, "memory_mib")
	sto, _ := numField(sz, "storage_gib")
	parts := []string{}
	if name != "" {
		parts = append(parts, name)
	}
	if cpu > 0 {
		parts = append(parts, fmt.Sprintf("%gm cpu", cpu))
	}
	if mem > 0 {
		parts = append(parts, fmt.Sprintf("%gMi ram", mem))
	}
	if sto > 0 {
		parts = append(parts, fmt.Sprintf("%gGi disk", sto))
	}
	return strings.Join(parts, " · ")
}

func numField(m map[string]any, k string) (float64, bool) {
	v, ok := m[k].(float64)
	return v, ok
}

// retentionForSchedule mirrors deployer.retentionForSchedule: anything firing
// multiple times a day keeps 7 days, everything else 30.
func retentionForSchedule(schedule string) int {
	if f := strings.Fields(schedule); len(f) >= 2 && f[1] == "*" {
		return 7
	}
	return 30
}

func init() {
	addonsGetCmd.Flags().BoolVar(&addonGetReveal, "reveal", false, "Show secret values and internal metadata in clear text")
	addonsCmd.AddCommand(addonsGetCmd)
}

// ---- summary columns for `machines addons list` ----------------------------
//
// Read from dedicated_config, which is empty for managed addons: those are
// backed by the shared platform instance and have no size, version or backup
// schedule of their own, so they render as "—" rather than a misleading value.

func addonDedicated(a api.ProjectAddon) map[string]any {
	if len(a.DedicatedConfig) == 0 {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(a.DedicatedConfig, &m) != nil {
		return nil
	}
	return m
}

func addonSizeCol(a api.ProjectAddon) string {
	m := addonDedicated(a)
	if m == nil {
		return "—"
	}
	if sz, ok := m["size"].(map[string]any); ok {
		if name, _ := sz["name"].(string); name != "" {
			return name
		}
	}
	if g, ok := numField(m, "storage_gb"); ok {
		return fmt.Sprintf("%gGB", g)
	}
	return "—"
}

func addonVersionCol(a api.ProjectAddon) string {
	m := addonDedicated(a)
	if m == nil {
		return "—"
	}
	if v, ok := m["version"].(string); ok && v != "" {
		return v
	}
	return "—"
}

// addonBackupCol mirrors the server's rule (UpdateAddonBackup): a per-addon
// backup schedule applies ONLY to a dedicated database. A managed database is
// backed up centrally by the platform, and every other addon type has no
// backup concept at all — so "off" would be wrong for all three, implying a
// switch the user could flip.
func addonBackupCol(a api.ProjectAddon) string {
	if a.AddonType != "database" {
		return "n/a"
	}
	if a.Mode != "dedicated" {
		return "central"
	}
	m := addonDedicated(a)
	if m == nil {
		return "off"
	}
	sched, _ := m["backup_schedule"].(string)
	if strings.TrimSpace(sched) == "" {
		return "off"
	}
	return sched
}

func addonUICol(a api.ProjectAddon) string {
	if !a.UIEnabled {
		return "—"
	}
	if u, ok := a.Config["_DBUI_URL"]; ok && u != "" {
		return "enabled"
	}
	return "enabled"
}

// ---- interactive `machines addons add` ------------------------------------

// addonAddWizard walks the catalogue and the dedicated-mode tunables, mirroring
// the dashboard's add-addon modal.
//
// Size presets are asked for as free text rather than offered as a list: the
// ladder lives on each provisioner server-side (AddonProvisioner.SizePresets)
// and no endpoint exposes it, so any list here would be a guess that silently
// rots. The server validates the name, and an unknown one is reported by it.
func addonAddWizard(client *api.Client, machineID string) error {
	catalog, err := client.ListProjectAddonsCatalog(machineID)
	if err != nil {
		return fmt.Errorf("load addon catalogue: %w", err)
	}
	if len(catalog) == 0 {
		return fmt.Errorf("addon catalogue is empty")
	}

	fmt.Println("Available addons:")
	for i, c := range catalog {
		mark := " "
		if c.InUse {
			mark = "*"
		}
		ui := ""
		if c.HasUI && c.UITool != "" {
			ui = output.Dim("  [" + c.UITool + "]")
		}
		if !c.SupportsDedicated {
			ui += output.Dim("  managed-only")
		}
		fmt.Printf("  %s %2d) %s %s%s\n", mark, i+1, output.Pad(output.Bold(c.Type), 13), c.Name, ui)
	}
	fmt.Println(output.Dim("  * already present in this machine (a second instance needs its own --name)"))

	// Accept either the number or the type name, since both are on screen.
	for {
		pick := ask("Addon (number or type)", "")
		if pick == "" {
			return fmt.Errorf("cancelled")
		}
		if n, cErr := strconv.Atoi(pick); cErr == nil {
			if n < 1 || n > len(catalog) {
				fmt.Printf("     ! choose 1-%d\n", len(catalog))
				continue
			}
			addonAddType = catalog[n-1].Type
			break
		}
		found := false
		for _, c := range catalog {
			if c.Type == pick {
				addonAddType, found = c.Type, true
				break
			}
		}
		if !found {
			fmt.Printf("     ! no addon type %q\n", pick)
			continue
		}
		break
	}

	// A second instance of a type needs its own name; the env keys are then
	// prefixed with it (ANALYTICS_DATABASE_URL rather than DATABASE_URL).
	inUse := false
	for _, c := range catalog {
		if c.Type == addonAddType && c.InUse {
			inUse = true
		}
	}
	if inUse {
		fmt.Println(output.Yellow("  this machine already has a " + addonAddType + " addon"))
		fmt.Println(output.Dim("  a second instance needs a name; its env keys get prefixed with it,"))
		fmt.Println(output.Dim("  e.g. name \"analytics\" → ANALYTICS_DATABASE_URL"))
		addonAddName = ask("Instance name", "")
		if addonAddName == "" {
			return fmt.Errorf("a second %s instance needs a name", addonAddType)
		}
	} else if n := ask("Instance name (blank = primary)", ""); n != "" {
		addonAddName = n
	}

	// Only ask about mode when the addon actually supports both. S3, cron and
	// oauth2 are managed-only, and offering "dedicated" there is a question
	// whose answer the server rejects.
	var chosen api.AddonCatalogEntry
	for _, c := range catalog {
		if c.Type == addonAddType {
			chosen = c.AddonCatalogEntry
		}
	}
	if !chosen.SupportsDedicated {
		addonAddMode = "managed"
		fmt.Println(output.Dim("  " + addonAddType + " is managed-only — wired to the shared platform instance (no quota cost)"))
	} else {
		addonAddMode = strings.ToLower(ask("Mode (managed/dedicated)", "managed"))
		if addonAddMode != "managed" && addonAddMode != "dedicated" {
			return fmt.Errorf("mode must be 'managed' or 'dedicated', got %q", addonAddMode)
		}
	}

	if addonAddMode == "dedicated" {
		fmt.Println(output.Dim("  dedicated runs inside the machine namespace and draws on its CPU/RAM quota"))
		if len(chosen.SizePresets) > 0 {
			for _, p := range chosen.SizePresets {
				fmt.Printf("    %s  %dm cpu · %dMi ram · %dGi disk\n",
					output.Pad(output.Bold(p.Name), 10), p.CPUMillis, p.MemoryMiB, p.StorageGiB)
			}
			addonAddSize = ask("Size preset", chosen.SizePresets[0].Name)
		} else {
			// Older API that does not publish the ladder (SizePresets was
			// added alongside SupportsDedicated). Do NOT name a preset in the
			// prompt: "(blank = smallest)" invited typing "smallest", and
			// ResolveAddonPreset accepts an unknown name at face value rather
			// than rejecting it — yielding a silently mis-sized addon.
			fmt.Println(output.Dim("  this API build does not publish the size ladder; leave blank for the default"))
			addonAddSize = ask("Size preset (press enter for default)", "")
		}
		if addonAddSize != "" && len(chosen.SizePresets) > 0 {
			valid := false
			names := make([]string, len(chosen.SizePresets))
			for i, p := range chosen.SizePresets {
				names[i] = p.Name
				if p.Name == addonAddSize {
					valid = true
				}
			}
			if !valid {
				return fmt.Errorf("unknown size preset %q — choose one of: %s",
					addonAddSize, strings.Join(names, ", "))
			}
		}
		addonAddVersion = ask("Version (blank = default)", "")
		if addonAddType == "database" {
			// Only a dedicated database supports a per-addon schedule; the
			// server rejects it for anything else.
			if sched := ask("Backup schedule, 5-field cron (blank = off)", ""); sched != "" {
				addonAddBackup = sched
			}
		}
	} else {
		fmt.Println(output.Dim("  managed wires the machine to the shared platform instance (no quota cost)"))
	}

	fmt.Printf("\n  %-10s %s\n", "Addon", addonAddType)
	if addonAddName != "" {
		fmt.Printf("  %-10s %s\n", "Name", addonAddName)
	}
	fmt.Printf("  %-10s %s\n", "Mode", addonAddMode)
	if addonAddSize != "" {
		fmt.Printf("  %-10s %s\n", "Size", addonAddSize)
	}
	if addonAddVersion != "" {
		fmt.Printf("  %-10s %s\n", "Version", addonAddVersion)
	}
	if addonAddBackup != "" {
		fmt.Printf("  %-10s %s\n", "Backups", addonAddBackup)
	}
	fmt.Println()
	if !confirm("Provision this addon?") {
		return fmt.Errorf("cancelled")
	}
	return nil
}
