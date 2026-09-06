package cmd

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
)

// The merged `usectl machines pods <machine>` view.
//
// A machine's pods come from three places and used to require three commands
// to see:
//
//   - the DECLARED config (repo, branch, domain, ports, limits, rollout
//     strategy, visibility) lives on project_apps and was only under `apps`;
//   - the RUNTIME pods (name, phase, node, restarts, owner) come from
//     GET /api/projects/{id}/pods and were only under `kpods`;
//   - DEDICATED ADDON pods (Postgres, Redis, NATS, ... StatefulSets) appear in
//     that same runtime listing but belong to no app at all, so nothing joined
//     them to the addon that owns them.
//
// This renders all three in one place. Runtime pods are matched to their
// declared pod by the `app` label, which is what the deployer selects on
// (deployer/k8s.go: MatchLabels{"app": appName}).

type podsView struct {
	// addonsByApp maps app id -> the addons whose env vars that pod receives.
	// Populated lazily: it costs one request per pod, so it is only filled
	// when the machine actually has addons to attach.
	addonsByApp map[string][]api.ProjectAddon
	apps        []api.ProjectApp
	addons      []api.ProjectAddon
	pods        []api.NamespacePod
	domains     []api.Domain
}

// podsJSON is the --json shape: declared config with its runtime instances
// nested underneath, rather than three disconnected arrays.
type podsJSON struct {
	Pods   []podJSON          `json:"pods"`
	Addons []addonPodJSON     `json:"addons"`
	Other  []api.NamespacePod `json:"other"`
}

type podJSON struct {
	api.ProjectApp
	// Domains resolves the per-pod domain pins, which the embedded
	// ProjectApp.Domain field no longer carries.
	Domains   []string           `json:"domains"`
	Addons    []api.ProjectAddon `json:"attached_addons"`
	Instances []api.NamespacePod `json:"instances"`
}

type addonPodJSON struct {
	api.ProjectAddon
	Instances []api.NamespacePod `json:"instances"`
}

func fetchPodsView(client *api.Client, machine string) (*podsView, error) {
	v := &podsView{}
	var err error
	if v.apps, err = client.ListProjectApps(machine); err != nil {
		return nil, err
	}
	// Addons and runtime pods are best-effort: a machine with no addons, or
	// one whose namespace has not been created yet, should still render its
	// declared pods rather than failing the whole command.
	v.addons, _ = client.ListProjectAddons(machine)
	v.pods, _ = client.ListNamespacePods(machine)
	// Keyed off the apps' project_id rather than the `machine` argument, which
	// may be a name while domain records carry the UUID.
	if len(v.apps) > 0 {
		v.domains, _ = client.ListProjectDomainRecords(v.apps[0].ProjectID)
	}
	v.addonsByApp = map[string][]api.ProjectAddon{}
	if len(v.addons) > 0 {
		for _, a := range v.apps {
			attached, aErr := client.ListAppAddonAttachments(machine, a.ID)
			if aErr == nil {
				v.addonsByApp[a.ID] = attached
			}
		}
	}
	return v, nil
}

// instancesFor returns the runtime pods carrying label app=<name>.
func (v *podsView) instancesFor(name string) []api.NamespacePod {
	var out []api.NamespacePod
	for _, p := range v.pods {
		if p.Labels["app"] == name {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// addonEngine maps an addon type to the token that appears in the names of
// the pods its provisioner creates ("database" addons run Postgres, so their
// pods are <machine>-postgres-...). Only types whose naming has actually been
// confirmed against a live cluster are listed; an unknown type simply falls
// through to OTHER PODS, which is honest rather than mislabelled.
var addonEngine = map[string]string{
	"database": "postgres",
}

// ordinalSuffix strips a StatefulSet ordinal ("-0", "-12") from a pod name.
var ordinalSuffix = regexp.MustCompile(`-\d+$`)

// addonInstances attributes still-unclaimed pods to the addon that owns them.
//
// Two rules learned from the live naming scheme
// (<machine>-<engine>[-<addonName>]-<ordinal>):
//
//   - a MANAGED addon never has pods of its own — it is backed by the shared
//     platform instance in kdeploy-infra — so it is skipped outright;
//   - a DEDICATED addon named something other than "primary" has that name as
//     the last segment before the ordinal.
//
// `claimed` must already contain the app pods. Without that, an addon and an
// app sharing a name (a "painboard" database next to a "painboard" web pod)
// would let the addon claim the app's ReplicaSet pod — which is exactly what
// a looser prefix match did.
func (v *podsView) addonInstances(a api.ProjectAddon, claimed map[string]bool) []api.NamespacePod {
	if a.Mode == "managed" {
		return nil
	}
	engine := addonEngine[a.AddonType]

	// Names of the other dedicated addons, so a "primary" addon does not
	// swallow pods that belong to a specifically-named sibling.
	siblings := map[string]bool{}
	for _, other := range v.addons {
		if other.Mode != "managed" && other.Name != "primary" && other.Name != a.Name {
			siblings[other.Name] = true
		}
	}

	var out []api.NamespacePod
	for _, p := range v.pods {
		if claimed[p.Namespace+"/"+p.Name] {
			continue
		}
		base := ordinalSuffix.ReplaceAllString(p.Name, "")
		segs := strings.Split(base, "-")
		last := segs[len(segs)-1]

		switch {
		case a.Name != "primary":
			if last != a.Name {
				continue
			}
		default:
			// The unnamed/default addon: require the engine token and make
			// sure the pod does not actually belong to a named sibling.
			if engine == "" || !strings.Contains(base, engine) || siblings[last] {
				continue
			}
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func runPodsView(machine string, wide bool) error {
	client, err := api.NewClient(apiURL)
	if err != nil {
		return err
	}
	// Accept a machine name, a UUID, or the ambient context (-m / env / use).
	ref, src, err := machineRef(machine)
	if err != nil {
		return err
	}
	echoMachineSource(ref, src)
	machineID, err := resolveMachine(client, ref)
	if err != nil {
		return err
	}
	v, err := fetchPodsView(client, machineID)
	if err != nil {
		return err
	}

	claimed := map[string]bool{}

	// App pods are claimed up front, before any addon is considered, so an
	// addon can never take a pod that belongs to an app of the same name.
	appInstances := make(map[string][]api.NamespacePod, len(v.apps))
	for _, a := range v.apps {
		inst := v.instancesFor(a.Name)
		appInstances[a.Name] = inst
		for _, p := range inst {
			claimed[p.Namespace+"/"+p.Name] = true
		}
	}

	if jsonOutput {
		out := podsJSON{Other: []api.NamespacePod{}}
		for _, a := range v.apps {
			out.Pods = append(out.Pods, podJSON{ProjectApp: a, Domains: v.domainsFor(a), Addons: v.addonsByApp[a.ID], Instances: appInstances[a.Name]})
		}
		for _, ad := range v.addons {
			inst := v.addonInstances(ad, claimed)
			for _, p := range inst {
				claimed[p.Namespace+"/"+p.Name] = true
			}
			out.Addons = append(out.Addons, addonPodJSON{ProjectAddon: ad, Instances: inst})
		}
		for _, p := range v.pods {
			if !claimed[p.Namespace+"/"+p.Name] {
				out.Other = append(out.Other, p)
			}
		}
		return output.JSON(out)
	}

	if len(v.apps) == 0 && len(v.addons) == 0 && len(v.pods) == 0 {
		fmt.Println("No pods in this machine.")
		fmt.Printf("\nAdd one:\n  usectl machines pods create %s --repo <url> --port <port>\n", machine)
		return nil
	}

	for _, a := range v.apps {
		printPodBlock(a, appInstances[a.Name], v.domainsFor(a), v.addonsByApp[a.ID], wide)
	}

	dedicated := make([]api.ProjectAddon, 0, len(v.addons))
	for _, ad := range v.addons {
		if len(v.addonInstances(ad, claimed)) > 0 {
			dedicated = append(dedicated, ad)
		}
	}
	if len(dedicated) > 0 {
		fmt.Println(output.Bold("ADDONS"))
		for _, ad := range dedicated {
			inst := v.addonInstances(ad, claimed)
			for _, p := range inst {
				claimed[p.Namespace+"/"+p.Name] = true
			}
			printAddonBlock(ad, inst)
		}
	}

	var other []api.NamespacePod
	for _, p := range v.pods {
		if !claimed[p.Namespace+"/"+p.Name] {
			other = append(other, p)
		}
	}
	if len(other) > 0 {
		// Cron Jobs, dbui, build pods — real pods spending the machine's
		// quota, so they are shown rather than silently dropped.
		fmt.Println(output.Bold("OTHER PODS"))
		rows := make([][]string, len(other))
		for i, p := range other {
			rows[i] = []string{p.Name, phaseColored(p), fmt.Sprintf("%d/%d", p.Ready, p.Total),
				fmt.Sprint(p.Restarts), output.Dim(p.NodeName), output.Dim(p.OwnerKind)}
		}
		output.Table([]string{"NAME", "PHASE", "READY", "RESTARTS", "NODE", "OWNER"}, rows)
	}
	return nil
}

func printPodBlock(a api.ProjectApp, inst []api.NamespacePod, domains []string, addons []api.ProjectAddon, wide bool) {
	fmt.Printf("%s  %s  %s\n", output.Bold(a.Name), podStateLabel(a, inst), output.Dim("("+shortID(a.ID)+")"))
	fmt.Printf("  %s %s\n", output.Dim(output.Pad("source", 11)), sourceLine(a))
	fmt.Printf("  %s %s\n", output.Dim(output.Pad("visibility", 11)), visibilityLine(a, domains))
	fmt.Printf("  %s %s\n", output.Dim(output.Pad("ports", 11)), portsLine(a))
	fmt.Printf("  %s %s\n", output.Dim(output.Pad("limits", 11)), limitsLine(a))
	fmt.Printf("  %s %s\n", output.Dim(output.Pad("rollout", 11)), rolloutLine(a))
	fmt.Printf("  %s %s\n", output.Dim(output.Pad("addons", 11)), addonsLine(addons))
	if a.Kind != "" && a.Kind != "web" {
		fmt.Printf("  %-11s %s\n", "kind", a.Kind)
	}
	if wide && a.Command != nil && *a.Command != "" {
		fmt.Printf("  %-11s %s\n", "command", *a.Command)
	}
	if len(inst) == 0 {
		fmt.Printf("  %s %s\n", output.Dim(output.Pad("instances", 11)), output.Dim("none running"))
	} else {
		fmt.Printf("  %s\n", output.Dim("instances"))
		for _, p := range inst {
			fmt.Printf("    %s %s %s  %s%s\n",
				output.Pad(p.Name, 42), output.Pad(phaseColored(p), 12),
				restartsColored(p.Restarts), output.Dim(orDash(p.NodeName)), reasonSuffix(p))
		}
	}
	fmt.Println()
}

func printAddonBlock(ad api.ProjectAddon, inst []api.NamespacePod) {
	mode := ad.Mode
	if mode == "" {
		mode = "dedicated"
	}
	fmt.Printf("%s  %s  %s\n", output.Bold(ad.Name), output.Cyan("["+ad.AddonType+" "+mode+"]"), output.Dim("("+shortID(ad.ID)+")"))
	for _, p := range inst {
		fmt.Printf("    %s %s %s  %s%s\n",
			output.Pad(p.Name, 42), output.Pad(phaseColored(p), 12),
			restartsColored(p.Restarts), output.Dim(orDash(p.NodeName)), reasonSuffix(p))
	}
	fmt.Println()
}

func sourceLine(a api.ProjectApp) string {
	// An image-sourced pod deploys a reference as-is: no repo, no branch, no
	// build. Reporting it as "—" (which reading RepoURL alone would do) hides
	// the only fact that matters about where the pod comes from.
	if a.SourceType == "image" || a.ImageRef != "" {
		return "image  " + a.ImageRef + "  (no build)"
	}
	if a.RepoURL == "" {
		return "—"
	}
	repo := strings.TrimSuffix(strings.TrimPrefix(a.RepoURL, "https://"), ".git")
	branch := a.Branch
	if branch == "" {
		branch = "(default)"
	}
	return fmt.Sprintf("git  %s @ %s", repo, branch)
}

// domainsFor returns the domains pinned to one pod. Domains live in their own
// table keyed by project_app_id (mig 022) — reading ProjectApp.Domain alone
// reports "no domain" for every machine created since, which is wrong.
func (v *podsView) domainsFor(a api.ProjectApp) []string {
	var out []string
	for _, d := range v.domains {
		if d.ProjectAppID != nil && *d.ProjectAppID == a.ID {
			name := d.Domain
			if !strings.Contains(name, ".") {
				name += ".usectl.com"
			}
			out = append(out, name)
		}
	}
	if len(out) == 0 && a.Domain != "" {
		out = append(out, a.Domain) // legacy single-domain machines
	}
	sort.Strings(out)
	return out
}

func visibilityLine(a api.ProjectApp, domains []string) string {
	if !a.IsPublic {
		return output.Yellow("internal") + output.Dim(" (ClusterIP only, no IngressRoute)")
	}
	if len(domains) == 0 {
		return output.Green("public") + output.Dim(" (no domain attached)")
	}
	return output.Green("public") + " → " + output.Cyan(strings.Join(domains, ", "))
}

func portsLine(a api.ProjectApp) string {
	parts := []string{fmt.Sprintf("%d primary", a.Port)}
	for _, p := range a.ExtraPorts {
		proto := p.Protocol
		if proto == "" {
			proto = "TCP"
		}
		parts = append(parts, fmt.Sprintf("%d/%s %s internal", p.Port, proto, p.Name))
	}
	if a.MetricsEnabled {
		mp := a.Port
		if a.MetricsPort != nil {
			mp = *a.MetricsPort
		}
		path := a.MetricsPath
		if path == "" {
			path = "/metrics"
		}
		parts = append(parts, fmt.Sprintf("%d%s metrics", mp, path))
	}
	return strings.Join(parts, " · ")
}

// limitsLine spells out when a value is the platform fallback rather than an
// explicit choice — "256Mi" and "256Mi (default)" are very different facts
// when you are trying to work out why a pod is being OOM-killed.
func limitsLine(a api.ProjectApp) string {
	cpu, mem, sto := "250m"+output.Dim(" (default)"), "256Mi"+output.Dim(" (default)"), "2Gi"+output.Dim(" (default)")
	if a.CPUMillis != nil {
		cpu = fmt.Sprintf("%dm", *a.CPUMillis)
	}
	if a.MemoryMiB != nil {
		mem = fmt.Sprintf("%dMi", *a.MemoryMiB)
	}
	if a.StorageMiB != nil {
		sto = fmt.Sprintf("%dMi", *a.StorageMiB)
	}
	return fmt.Sprintf("%s cpu · %s ram · %s storage", cpu, mem, sto)
}

// addonsLine lists the addons injecting env vars into this pod. "none" is
// worth stating explicitly: a machine can own a database that a given pod
// cannot see, and that asymmetry is the usual cause of a pod that starts but
// cannot connect to anything.
func addonsLine(addons []api.ProjectAddon) string {
	if len(addons) == 0 {
		// NOT the same as "receives nothing": a pod with no attachment rows
		// inherits every addon in the machine (deployer.perAppAddonSecrets
		// falls back to the project-wide list). Saying "none attached" alone
		// reads as "no addon variables reach this pod", which is the opposite
		// of what happens.
		return output.Yellow("none pinned") + output.Dim(" — inherits every addon in the machine")
	}
	parts := make([]string, len(addons))
	for i, a := range addons {
		parts[i] = a.AddonType + "/" + a.Name
	}
	sort.Strings(parts)
	return strings.Join(parts, " · ")
}

func rolloutLine(a api.ProjectApp) string {
	if a.RolloutStrategy == nil || *a.RolloutStrategy == "" {
		return "rolling" + output.Dim(" (default, 25% surge)")
	}
	if *a.RolloutStrategy == "recreate" {
		return output.Yellow("recreate") + output.Dim(" (terminate then create; brief downtime)")
	}
	return *a.RolloutStrategy
}

func podStateLabel(a api.ProjectApp, inst []api.NamespacePod) string {
	if a.IsStopped {
		return output.Yellow("○ stopped")
	}
	if len(inst) == 0 {
		return output.Dim("○ not running")
	}
	ready := 0
	for _, p := range inst {
		if p.Ready == p.Total && p.Total > 0 && !p.Terminating {
			ready++
		}
	}
	if ready == len(inst) {
		return output.Green(fmt.Sprintf("● running %d/%d", ready, len(inst)))
	}
	if ready == 0 {
		return output.Red(fmt.Sprintf("● down %d/%d", ready, len(inst)))
	}
	return output.Yellow(fmt.Sprintf("◐ degraded %d/%d", ready, len(inst)))
}

// phaseColored maps a pod phase to a colour: green only for a pod that is both
// Running and fully ready, red for terminal failure, amber for anything still
// in motion. A Running-but-not-ready pod is deliberately NOT green — that is
// the state a crash-looping container sits in between restarts.
func phaseColored(p api.NamespacePod) string {
	txt := podPhase(p)
	switch {
	case p.Terminating:
		return output.Yellow(txt)
	case txt == "Running" && p.Ready == p.Total && p.Total > 0:
		return output.Green(txt)
	case txt == "Failed", txt == "Unknown":
		return output.Red(txt)
	case txt == "Succeeded":
		return output.Dim(txt)
	default:
		return output.Yellow(txt)
	}
}

// restartsColored highlights restart counts, which are the single most useful
// early signal that something is wrong.
func restartsColored(n int32) string {
	txt := fmt.Sprintf("%2d restart(s)", n)
	switch {
	case n == 0:
		return output.Dim(txt)
	case n < 5:
		return output.Yellow(txt)
	default:
		return output.Red(txt)
	}
}

// podPhase reports Terminating explicitly: K8s leaves phase at Running for the
// whole grace period, which reads as healthy for a pod on its way out.
func podPhase(p api.NamespacePod) string {
	if p.Terminating {
		return "Terminating"
	}
	return p.Phase
}

func reasonSuffix(p api.NamespacePod) string {
	if p.Reason == "" {
		return ""
	}
	return "  " + output.Red(p.Reason)
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
