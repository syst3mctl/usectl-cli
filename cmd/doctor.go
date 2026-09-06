package cmd

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

// `usectl machines doctor` — the answer to "the pod is up but it cannot see
// the database that is obviously right there".
//
// `machines diagnostics` only ever reports on ONE pod and only reads back what
// Kubernetes already noticed: crash reasons, restart counts, events. The most
// common misconfiguration in this platform produces none of those signals. A
// pod with no rows in project_app_addons starts perfectly, stays Running, logs
// nothing unusual, and simply has no DATABASE_URL. K8s has no opinion about
// it, so no amount of event-reading will surface it.
//
// This command checks the declared configuration instead of the runtime, so it
// catches the class of problem that is invisible to diagnostics. It is
// read-only and runs entirely against endpoints that already exist.

var (
	doctorStrict bool
	doctorPod    string
)

// Severity ordering is used for both the summary counts and the exit code.
const (
	lvlError = "error"
	lvlWarn  = "warn"
)

// finding is one problem found on one pod (or on the machine as a whole, when
// Pod is empty). Fix carries a runnable command rather than prose — a finding
// the user cannot act on is just noise.
type finding struct {
	Level  string `json:"level"`
	Pod    string `json:"pod,omitempty"`
	Check  string `json:"check"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

type doctorReport struct {
	Machine  string      `json:"machine"`
	Pods     int         `json:"pods"`
	Addons   int         `json:"addons"`
	Usage    []usageStat `json:"usage"`
	Findings []finding   `json:"findings"`
	Errors   int         `json:"errors"`
	Warnings int         `json:"warnings"`
}

// usageStat is one running instance's consumption next to the limit it is
// measured against. Both halves matter: "300 MB" means nothing on its own,
// and a percentage alone hides which of two equally-loaded pods is the one
// with the small limit.
type usageStat struct {
	Pod      string  `json:"pod"`
	Instance string  `json:"instance"`
	CPUUsed  float64 `json:"cpu_millis_used"`
	CPULimit int     `json:"cpu_millis_limit"`
	MemUsed  int64   `json:"memory_bytes_used"`
	MemLimit int     `json:"memory_mib_limit"`
	StoUsed  int64   `json:"storage_bytes_used"`
	StoLimit int     `json:"storage_mib_limit"`
	// LimitsExplicit is false when the pod never set its own limits and the
	// platform default applies. Percentages are still shown, but no pressure
	// finding is raised off a limit the CLI only assumes.
	LimitsExplicit bool `json:"limits_explicit"`
	// Sampled is false when the kubelet reported nothing for this instance.
	// Zero usage and "no sample" look identical in the numbers and must not.
	Sampled bool `json:"sampled"`
}

var doctorCmd = &cobra.Command{
	Use:     "doctor [machine]",
	Aliases: []string{"check"},
	Short:   "Check a machine for misconfigurations that do not show up as crashes",
	Long: `Audits every pod in a machine for problems that leave the pod running and
therefore never appear in 'machines diagnostics' or in the logs.

The headline check is addon attachment. Provisioning an addon does NOT wire it
into a pod — a pod only receives DATABASE_URL, REDIS_URL, S3_* and friends for
the addons explicitly attached to it. A pod created without '--addon' has none,
starts fine, and fails at the first query with a connection error that looks
like a network problem. Adding an addon to the machine later does not reach
pods that already exist either.

Configuration checks:

  addons-unattached  the machine has addons but this pod is attached to none,
                     so no addon credentials are injected
  addon-failed       an attached addon never finished provisioning
  addon-stopped      an attached addon is scaled to zero; the credentials are
                     injected but nothing is listening on the other end
  addon-provisioning an attached addon is still coming up
  no-domain          a public web pod has no domain, so nothing outside the
                     cluster can reach it
  addon-orphan       (machine-level) an addon is attached to no pod at all —
                     provisioned and billed, reachable by nothing

Runtime checks:

  oomkilled          the container exceeded its memory limit and was killed.
                     Reported even after the restart, when every phase-based
                     view has gone back to showing "Running"
  crashloop          the container exits on startup (CrashLoopBackOff)
  image-pull         the image cannot be pulled — wrong tag or missing
                     registry credentials
  container-config   a Secret or ConfigMap named in the pod is missing from
                     the namespace, so the container never starts
  container-start    the runtime refused to start the container
  unschedulable      no node has room for the pod, or none matches its
                     selector
  evicted            the node reclaimed the pod under memory or disk pressure
  stuck-pending      Pending for over ten minutes — never scheduled
  exited-nonzero     the container last exited with a non-zero code
  restarting         3+ restarts with no reason recorded anywhere
  not-ready          Running but failing its readiness probe, so it is in no
                     Service endpoint and receives no traffic
  not-running        the pod is not stopped, yet has no running instance
  deadline-exceeded  terminated for exceeding its active deadline

Usage checks:

  memory-pressure    working set is at 85% (warn) or 95% (error) of the pod's
                     memory limit — the next allocation gets it OOM-killed
  storage-pressure   ephemeral storage is near the pod's limit; a pod that
                     goes over is evicted, not throttled

Every run also prints a usage table: CPU, memory and ephemeral storage per
running instance, each against the limit it is measured by. CPU is shown but
never judged — exceeding a CPU limit throttles the container rather than
killing it, and a single kubelet sample taken mid-request routinely reads at
100% on a perfectly healthy pod. Memory and storage limits are enforced by
eviction, so those are the ones that carry a finding.

Pressure findings are raised only against limits the pod sets itself. A pod
running on the platform default still shows its percentages, but no finding is
raised off a limit this CLI only assumed.

Each check is reported once per pod, naming the first instance that tripped it,
so a pod with ten replicas does not print ten identical lines.

Read-only: it changes nothing and only reads endpoints you already have access
to. Exit status is 0 unless --strict is passed.`,
	Example: `  usectl machines doctor
  usectl machines doctor my-machine
  usectl machines doctor my-machine --pod web
  usectl machines doctor my-machine --json
  usectl machines doctor my-machine --strict   # exit 1 when an error is found`,
	Args: cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		// Keep what the user typed for the fix hints: echoing back a UUID they
		// never used is a worse copy-paste than the name they already know.
		typed := ""
		if len(args) > 0 {
			typed = args[0]
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}
		machineID := args[0]
		label := typed
		if label == "" {
			label = machineID
		}

		v, err := fetchPodsView(client, machineID)
		if err != nil {
			return err
		}

		if doctorPod != "" {
			// Filter to the named pod. Done here rather than server-side
			// because fetchPodsView is a machine-scoped view and the addon
			// orphan check still needs to see every pod's attachments.
			if !v.hasPod(doctorPod) {
				return fmt.Errorf("no pod named %q in this machine", doctorPod)
			}
		}

		// Usage is best-effort: it comes from the kubelet summary API via the
		// backend, so a machine whose nodes are unreachable, or a cluster with
		// no metrics at all, should still get its configuration audited rather
		// than fail the whole command.
		usage := map[string]api.PodStats{}
		if st, sErr := client.GetProjectStats(machineID); sErr == nil {
			for _, ps := range st.Pods {
				usage[ps.Name] = ps
			}
		}

		rep := doctorReport{Machine: label, Pods: len(v.apps), Addons: len(v.addons)}
		rep.Usage = collectUsage(v, usage)
		rep.Findings = runDoctorChecks(v, usage, label)
		if doctorPod != "" {
			kept := rep.Findings[:0]
			for _, f := range rep.Findings {
				if f.Pod == doctorPod {
					kept = append(kept, f)
				}
			}
			rep.Findings = kept
			keptU := rep.Usage[:0]
			for _, u := range rep.Usage {
				if u.Pod == doctorPod {
					keptU = append(keptU, u)
				}
			}
			rep.Usage = keptU
		}
		for _, f := range rep.Findings {
			if f.Level == lvlError {
				rep.Errors++
			} else {
				rep.Warnings++
			}
		}

		if jsonOutput {
			if err := output.JSON(rep); err != nil {
				return err
			}
			if doctorStrict && rep.Errors > 0 {
				return doctorStrictErr(rep.Errors)
			}
			return nil
		}

		printDoctorReport(v, rep)
		if doctorStrict && rep.Errors > 0 {
			return doctorStrictErr(rep.Errors)
		}
		return nil
	},
	// The findings are the output; a cobra usage dump on --strict failure
	// would bury them.
	SilenceUsage: true,
}

// doctorStrictErr is what --strict returns so the process exits 1. The text
// stays short because the report above it already said everything; this line
// exists to make the failure legible in CI output, not to repeat the findings.
func doctorStrictErr(n int) error {
	return fmt.Errorf("doctor: %d error-level problem(s) found", n)
}

func runDoctorChecks(v *podsView, usage map[string]api.PodStats, machine string) []finding {
	var out []finding

	// Which addons are reachable from at least one pod. Filled while walking
	// the pods so the machine-level orphan check below costs no extra request.
	used := map[string]bool{}

	for _, a := range v.apps {
		attached := v.addonsByApp[a.ID]
		for _, ad := range attached {
			used[ad.ID] = true
		}

		// The headline check. Only meaningful when the machine actually has
		// addons — a machine with none is correctly configured with none.
		if len(v.addons) > 0 && len(attached) == 0 {
			names := make([]string, len(v.addons))
			for i, ad := range v.addons {
				names[i] = ad.AddonType + "/" + ad.Name
			}
			sort.Strings(names)
			out = append(out, finding{
				Level: lvlError,
				Pod:   a.Name,
				Check: "addons-unattached",
				Detail: fmt.Sprintf(
					"machine has %d addon(s) (%s) but none is attached to this pod — it receives no addon credentials",
					len(v.addons), strings.Join(names, ", ")),
				Fix: fmt.Sprintf("usectl machines pods attach-addon %s %s --all", machine, a.Name),
			})
		}

		for _, ad := range attached {
			ref := ad.AddonType + "/" + ad.Name
			switch {
			case ad.Status == "failed":
				out = append(out, finding{
					Level:  lvlError,
					Pod:    a.Name,
					Check:  "addon-failed",
					Detail: fmt.Sprintf("attached addon %s is in status 'failed' — its credentials may be absent or stale", ref),
					Fix:    fmt.Sprintf("usectl machines addons list %s", machine),
				})
			case ad.IsStopped || ad.Status == "stopped":
				out = append(out, finding{
					Level:  lvlWarn,
					Pod:    a.Name,
					Check:  "addon-stopped",
					Detail: fmt.Sprintf("attached addon %s is scaled to zero — credentials are injected but nothing is listening", ref),
					Fix:    fmt.Sprintf("usectl machines addons start %s %s", machine, ad.ID),
				})
			case ad.Status == "pending" || ad.Status == "provisioning":
				out = append(out, finding{
					Level:  lvlWarn,
					Pod:    a.Name,
					Check:  "addon-provisioning",
					Detail: fmt.Sprintf("attached addon %s is still %s — credentials are not final yet", ref, ad.Status),
				})
			}
		}

		// Runtime checks are meaningless for a pod the user deliberately
		// stopped, and reporting them would train people to ignore the report.
		if a.IsStopped {
			continue
		}

		inst := v.instancesFor(a.Name)
		if len(inst) == 0 {
			out = append(out, finding{
				Level:  lvlWarn,
				Pod:    a.Name,
				Check:  "not-running",
				Detail: "pod is not stopped but has no running instance — it may never have been deployed",
				Fix:    fmt.Sprintf("usectl machines deploy %s", machine),
			})
		}
		out = append(out, instanceFindings(a.Name, inst, machine)...)
		out = append(out, pressureFindings(a, inst, usage, machine)...)

		// A private pod with no domain is correct by construction; only a
		// public one is broken by the absence.
		if a.IsPublic && (a.Kind == "" || a.Kind == "web") && len(v.domainsFor(a)) == 0 {
			out = append(out, finding{
				Level:  lvlWarn,
				Pod:    a.Name,
				Check:  "no-domain",
				Detail: "pod is public but has no domain attached — nothing outside the cluster can reach it",
				Fix:    fmt.Sprintf("usectl domains list --project %s", machine),
			})
		}
	}

	// Machine-level: an addon nothing is attached to. Costs real money and
	// quota while being reachable by no pod.
	for _, ad := range v.addons {
		if used[ad.ID] || len(v.apps) == 0 {
			continue
		}
		out = append(out, finding{
			Level:  lvlWarn,
			Check:  "addon-orphan",
			Detail: fmt.Sprintf("addon %s/%s is attached to no pod — it is provisioned and billed but nothing can reach it", ad.AddonType, ad.Name),
			Fix:    fmt.Sprintf("usectl machines pods attach-addon %s <pod> %s/%s", machine, ad.AddonType, ad.Name),
		})
	}

	return out
}

// Platform defaults applied by the namespace LimitRange when a pod never set
// its own limits. Mirrors what `machines usage` prints as "(def)". Used only
// to render a percentage — never to raise a finding, because a default that
// drifts from the cluster's LimitRange would produce confident nonsense.
const (
	defaultCPUMillis = 250
	defaultMemMiB    = 256
	defaultStoMiB    = 2048
)

// Pressure thresholds. A pod is not "nearly out of memory" at 80% — normal
// heap behaviour lives up there. These are set where the next allocation
// plausibly kills the container, so that a finding means something.
const (
	pressureWarnPct  = 85.0
	pressureErrorPct = 95.0
)

// collectUsage joins each running instance's sampled usage to the limit its
// pod declares. Instances come from the pods view (matched by the `app`
// label) and usage from the stats endpoint (keyed by pod name), so the two
// are joined on the exact instance name rather than a name prefix — "web"
// and "web-api" would otherwise share a prefix and swap numbers.
func collectUsage(v *podsView, usage map[string]api.PodStats) []usageStat {
	var out []usageStat
	for _, a := range v.apps {
		explicit := a.CPUMillis != nil || a.MemoryMiB != nil || a.StorageMiB != nil
		for _, p := range v.instancesFor(a.Name) {
			u := usageStat{
				Pod:            a.Name,
				Instance:       p.Name,
				CPULimit:       intOr(a.CPUMillis, defaultCPUMillis),
				MemLimit:       intOr(a.MemoryMiB, defaultMemMiB),
				StoLimit:       intOr(a.StorageMiB, defaultStoMiB),
				LimitsExplicit: explicit,
			}
			if ps, ok := usage[p.Name]; ok {
				// A pod the kubelet reported on always has SOME cpu or memory
				// sample; all-zero means the sample never arrived, which is
				// not the same as an idle pod and must not render as 0%.
				u.Sampled = ps.CPUMillis > 0 || ps.MemoryBytes > 0 || ps.StorageBytes > 0
				u.CPUUsed = ps.CPUMillis
				u.MemUsed = ps.MemoryBytes
				u.StoUsed = ps.StorageBytes
			}
			out = append(out, u)
		}
	}
	return out
}

// pressureFindings raises the usage-derived problems: a pod about to be
// OOM-killed, and one about to be evicted for filling its ephemeral disk.
//
// Deliberately not checked: CPU. Exceeding a CPU limit throttles the
// container, it does not kill it, and the kubelet sample is instantaneous —
// a pod mid-request routinely reads at 100% of its limit for one sample and
// is perfectly healthy. Alerting on that would train people to ignore the
// report, so CPU is shown and left unjudged.
func pressureFindings(a api.ProjectApp, inst []api.NamespacePod, usage map[string]api.PodStats, machine string) []finding {
	var out []finding
	if a.CPUMillis == nil && a.MemoryMiB == nil && a.StorageMiB == nil {
		// Every limit is the platform default. Percentages still render in
		// the usage table; a finding would be asserting a number this CLI
		// only assumed.
		return nil
	}
	seen := map[string]bool{}
	for _, p := range inst {
		ps, ok := usage[p.Name]
		if !ok {
			continue
		}
		if a.MemoryMiB != nil && ps.MemoryBytes > 0 {
			pct := float64(ps.MemoryBytes) / float64(int64(*a.MemoryMiB)<<20) * 100
			if pct >= pressureWarnPct && !seen["memory-pressure"] {
				seen["memory-pressure"] = true
				lvl := lvlWarn
				tail := "— the next allocation may get it OOM-killed"
				if pct >= pressureErrorPct {
					lvl = lvlError
					tail = "— an OOM kill is imminent"
				}
				out = append(out, finding{
					Level: lvl, Pod: a.Name, Check: "memory-pressure",
					Detail: fmt.Sprintf("instance %s is using %s of its %dMi memory limit (%.0f%%) %s",
						p.Name, humanBytes(ps.MemoryBytes), *a.MemoryMiB, pct, tail),
					Fix: fmt.Sprintf("usectl machines pods set %s %s memory=%dMi", machine, a.Name, *a.MemoryMiB*2),
				})
			}
		}
		if a.StorageMiB != nil && ps.StorageBytes > 0 {
			pct := float64(ps.StorageBytes) / float64(int64(*a.StorageMiB)<<20) * 100
			if pct >= pressureWarnPct && !seen["storage-pressure"] {
				seen["storage-pressure"] = true
				lvl := lvlWarn
				if pct >= pressureErrorPct {
					lvl = lvlError
				}
				out = append(out, finding{
					Level: lvl, Pod: a.Name, Check: "storage-pressure",
					Detail: fmt.Sprintf("instance %s is using %s of its %dMi ephemeral storage limit (%.0f%%) — a pod over its limit is evicted, not throttled",
						p.Name, humanBytes(ps.StorageBytes), *a.StorageMiB, pct),
					Fix: fmt.Sprintf("usectl machines pods set %s %s storage=%dMi", machine, a.Name, *a.StorageMiB*2),
				})
			}
		}
	}
	return out
}

// instanceFindings inspects the running instances of one pod.
//
// Two things make this less obvious than reading a phase:
//
//   - The reason a container is unhealthy NOW (`Reason`) and the reason it
//     died LAST time (`LastTerminationReason`) are different fields, and the
//     second is the one that catches an OOM kill. A container the kubelet
//     killed for exceeding its memory limit is restarted immediately and
//     reports Running with an empty Reason; only the last-termination record
//     still says OOMKilled. Checking phase alone reports that pod as fine
//     while it is in fact being killed on a loop.
//   - A pod with N replicas would otherwise emit N copies of the same
//     finding. Each check is reported once per pod, naming the first
//     instance that tripped it.
func instanceFindings(podName string, inst []api.NamespacePod, machine string) []finding {
	var out []finding
	seen := map[string]bool{}
	add := func(level, check, detail, fix string) {
		if seen[check] {
			return
		}
		seen[check] = true
		out = append(out, finding{Level: level, Pod: podName, Check: check, Detail: detail, Fix: fix})
	}

	for _, p := range inst {
		// Killed for exceeding its memory limit. Worth its own check because
		// the symptom people describe ("it restarts every few minutes") is
		// indistinguishable from an application crash, and the fix is
		// completely different.
		if p.LastTerminationReason == "OOMKilled" {
			add(lvlError, "oomkilled",
				fmt.Sprintf("instance %s was OOM-killed — the container exceeded its memory limit and the kernel killed it (%d restart(s) so far)", p.Name, p.Restarts),
				fmt.Sprintf("usectl machines pods set %s %s memory=<larger>", machine, podName))
		}

		switch {
		case strings.Contains(p.Reason, "CrashLoop"):
			add(lvlError, "crashloop",
				fmt.Sprintf("instance %s is in CrashLoopBackOff after %d restart(s) — it exits on startup", p.Name, p.Restarts),
				fmt.Sprintf("usectl machines diagnostics %s", machine))
		case p.Reason == "ImagePullBackOff" || p.Reason == "ErrImagePull":
			add(lvlError, "image-pull",
				fmt.Sprintf("instance %s cannot pull its image — wrong tag, or the registry credentials are missing", p.Name),
				fmt.Sprintf("usectl machines deployments %s", machine))
		case p.Reason == "CreateContainerConfigError":
			// Almost always a Secret or ConfigMap named in envFrom that does
			// not exist in the namespace — which is exactly the shape an
			// addon problem takes at pod-start time.
			add(lvlError, "container-config",
				fmt.Sprintf("instance %s cannot start: a Secret or ConfigMap it references is missing from the namespace — often an addon that was removed or never provisioned", p.Name),
				fmt.Sprintf("usectl machines pods addons %s %s", machine, podName))
		case p.Reason == "CreateContainerError" || p.Reason == "ContainerCannotRun":
			add(lvlError, "container-start",
				fmt.Sprintf("instance %s could not be started by the runtime: %s", p.Name, orDash(p.Message)),
				fmt.Sprintf("usectl machines diagnostics %s", machine))
		case p.Reason == "Unschedulable":
			add(lvlError, "unschedulable",
				fmt.Sprintf("instance %s cannot be placed on any node — not enough CPU/memory left, or nothing matches its node selector", p.Name),
				fmt.Sprintf("usectl machines quota %s", machine))
		case p.Reason == "Evicted":
			add(lvlError, "evicted",
				fmt.Sprintf("instance %s was evicted from its node, usually under memory or disk pressure", p.Name),
				fmt.Sprintf("usectl machines diagnostics %s", machine))
		case p.Reason == "DeadlineExceeded":
			add(lvlWarn, "deadline-exceeded",
				fmt.Sprintf("instance %s was terminated for exceeding its active deadline", p.Name), "")
		}

		// A non-zero exit that was not an OOM and has not reached
		// CrashLoopBackOff yet: still worth surfacing, because a container
		// that restarts cleanly every few minutes looks healthy in a phase
		// listing.
		if p.LastTerminationReason != "" && p.LastTerminationReason != "OOMKilled" &&
			p.LastTerminationReason != "Completed" && p.LastTerminationCode != 0 {
			add(lvlWarn, "exited-nonzero",
				fmt.Sprintf("instance %s last exited with code %d (%s) — it has restarted %d time(s)",
					p.Name, p.LastTerminationCode, p.LastTerminationReason, p.Restarts),
				fmt.Sprintf("usectl machines pods logs %s %s", machine, podName))
		}

		// Restarting repeatedly with no reason recorded anywhere. Kept last
		// so a named cause above always wins the explanation.
		if p.Restarts >= 3 {
			add(lvlWarn, "restarting",
				fmt.Sprintf("instance %s has restarted %d time(s) with no reason recorded", p.Name, p.Restarts),
				fmt.Sprintf("usectl machines pods logs %s %s", machine, podName))
		}

		// Running but never became ready: the process is up and the
		// readiness probe is failing, which serves no traffic while looking
		// alive in every phase-based view.
		if p.Phase == "Running" && p.Total > 0 && p.Ready < p.Total && !p.Terminating {
			add(lvlWarn, "not-ready",
				fmt.Sprintf("instance %s is Running but only %d/%d container(s) are ready — failing its readiness probe, so it receives no traffic", p.Name, p.Ready, p.Total),
				fmt.Sprintf("usectl machines pods logs %s %s", machine, podName))
		}

		// Stuck Pending. Short-lived Pending is normal during a rollout, so
		// only flag it once it has clearly stopped making progress.
		if p.Phase == "Pending" && !p.CreatedAt.IsZero() && time.Since(p.CreatedAt) > 10*time.Minute {
			add(lvlError, "stuck-pending",
				fmt.Sprintf("instance %s has been Pending for %s — it was never scheduled onto a node", p.Name, time.Since(p.CreatedAt).Round(time.Minute)),
				fmt.Sprintf("usectl machines quota %s", machine))
		}
	}
	return out
}

// printUsageTable shows what each running instance is actually consuming
// against the limit it is measured by. Usage is not a "problem" and so is not
// a finding — but it is the first thing anyone wants after being told a pod
// was OOM-killed, and having to run a second command to get it is what made
// the kill look mysterious in the first place.
func printUsageTable(us []usageStat) {
	if len(us) == 0 {
		return
	}
	rows := make([][]string, 0, len(us))
	for _, u := range us {
		if !u.Sampled {
			// No sample is its own answer. Rendering it as 0% would say the
			// pod is idle, which is a different and possibly false claim.
			rows = append(rows, []string{u.Pod, u.Instance,
				output.Dim("no sample"), output.Dim("no sample"), output.Dim("no sample")})
			continue
		}
		rows = append(rows, []string{
			u.Pod, u.Instance,
			fmt.Sprintf("%.0fm / %dm%s", u.CPUUsed, u.CPULimit, pctSuffix(u.CPUUsed, float64(u.CPULimit), false)),
			fmt.Sprintf("%s / %dMi%s", humanBytes(u.MemUsed), u.MemLimit, pctSuffix(float64(u.MemUsed), float64(int64(u.MemLimit)<<20), true)),
			fmt.Sprintf("%s / %dMi%s", humanBytes(u.StoUsed), u.StoLimit, pctSuffix(float64(u.StoUsed), float64(int64(u.StoLimit)<<20), true)),
		})
	}
	fmt.Printf("%s\n", output.Bold("usage"))
	output.Table([]string{"POD", "INSTANCE", "CPU", "MEMORY", "STORAGE"}, rows)
	if anyDefaulted(us) {
		fmt.Printf("%s\n", output.Dim("  limits marked against platform defaults where the pod sets none"))
	}
	fmt.Println()
}

// pctSuffix renders the percentage, coloured only where crossing it has a
// consequence. CPU is never coloured: going over a CPU limit throttles,
// going over a memory or storage limit kills the pod.
func pctSuffix(used, limit float64, enforced bool) string {
	if limit <= 0 {
		return ""
	}
	pct := used / limit * 100
	txt := fmt.Sprintf(" (%.0f%%)", pct)
	if !enforced {
		return output.Dim(txt)
	}
	switch {
	case pct >= pressureErrorPct:
		return output.Red(txt)
	case pct >= pressureWarnPct:
		return output.Yellow(txt)
	}
	return output.Dim(txt)
}

func anyDefaulted(us []usageStat) bool {
	for _, u := range us {
		if !u.LimitsExplicit {
			return true
		}
	}
	return false
}

func printDoctorReport(v *podsView, rep doctorReport) {
	fmt.Printf("%s  %s\n", output.Bold("doctor"), output.Dim(rep.Machine))
	fmt.Printf("%s\n\n", output.Dim(fmt.Sprintf("%d pod(s) · %d addon(s)", rep.Pods, rep.Addons)))

	printUsageTable(rep.Usage)

	if len(rep.Findings) == 0 {
		fmt.Printf("%s no problems found\n", output.Green("✓"))
		return
	}

	// Group by pod so the output reads per-pod rather than per-check, with
	// machine-level findings last under their own heading.
	byPod := map[string][]finding{}
	var order []string
	for _, f := range rep.Findings {
		key := f.Pod
		if _, seen := byPod[key]; !seen {
			order = append(order, key)
		}
		byPod[key] = append(byPod[key], f)
	}
	sort.SliceStable(order, func(i, j int) bool {
		// Machine-level ("") sorts last.
		if (order[i] == "") != (order[j] == "") {
			return order[j] == ""
		}
		return false
	})

	for _, key := range order {
		if key == "" {
			fmt.Printf("%s\n", output.Bold("machine"))
		} else {
			fmt.Printf("%s\n", output.Bold(key))
		}
		for _, f := range byPod[key] {
			mark := output.Yellow("⚠")
			if f.Level == lvlError {
				mark = output.Red("✗")
			}
			fmt.Printf("  %s %s %s\n", mark, output.Pad(f.Check, 20), f.Detail)
			if f.Fix != "" {
				fmt.Printf("    %s %s\n", output.Dim("fix"), f.Fix)
			}
		}
		fmt.Println()
	}

	// Pods that passed everything are worth naming: a report that only lists
	// problems leaves the user unsure whether the rest was even checked.
	var clean []string
	for _, a := range v.apps {
		if len(byPod[a.Name]) == 0 {
			clean = append(clean, a.Name)
		}
	}
	if len(clean) > 0 {
		sort.Strings(clean)
		fmt.Printf("%s %s\n\n", output.Green("✓"), output.Dim("no problems: "+strings.Join(clean, ", ")))
	}

	fmt.Printf("%d problem(s): %s, %s\n",
		len(rep.Findings),
		output.Red(fmt.Sprintf("%d error", rep.Errors)),
		output.Yellow(fmt.Sprintf("%d warning", rep.Warnings)))
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorStrict, "strict", false, "Exit with status 1 when an error-level problem is found")
	doctorCmd.Flags().StringVar(&doctorPod, "pod", "", "Limit the report to one pod")
	projectsCmd.AddCommand(doctorCmd)
}
