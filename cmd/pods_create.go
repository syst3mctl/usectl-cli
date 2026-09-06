package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

// `usectl machines pods create` — add a workload to a machine.
//
// A pod comes from ONE of two sources, and neither is more "normal" than the
// other:
//
//	--repo   build from a Git repository with Kaniko
//	--image  deploy a prebuilt reference as-is; no GitHub involvement at all
//
// The image path needs no repo, no branch, no GitHub App installation and no
// build — useful for third-party images (redis:7, grafana/grafana) and for
// images built by someone else's CI.
//
// --addon attaches addons in the same step. Attaching is what injects an
// addon's credentials into the pod as env vars (DATABASE_URL, REDIS_URL, ...);
// a pod that is not attached to an addon cannot see it, which is the usual
// reason a freshly created pod cannot reach its database.

var (
	podCreateName       string
	podCreateRepo       string
	podCreateImage      string
	podCreateRegUser    string
	podCreateRegPass    string
	podCreateBranch     string
	podCreateDomain     string
	podCreatePort       int
	podCreateReplicas   int
	podCreatePrivate    bool
	podCreateKind       string
	podCreateCommand    string
	podCreateExtraPorts []string
	podCreateArgs       []string
	podCreateAddons     []string
	podCreateAllAddons  bool
	podCreateAutoDeploy bool
	podCreateInstallID  int64
)

var podsCreateCmd = &cobra.Command{
	Use:   "create <machine> [name]",
	Short: "Add a pod to a machine, from a Git repo or a prebuilt image",
	Long: `Create a workload inside an existing machine.

A pod is built from EITHER a Git repository or a prebuilt container image:

  --repo   https://github.com/me/api      builds with Kaniko on every push
  --image  ghcr.io/acme/api:v1.2.3        deploys as-is, no GitHub, no build

The image source needs no repository, no branch and no GitHub App — pass
--registry-user / --registry-password for a private registry.

Addons are attached with --addon, which is what injects their credentials
into the pod as environment variables. A pod with no addon attached cannot
see the machine's database even though the database exists.

Machines, pods and addons may all be named rather than passed as UUIDs.

Run with no flags on a terminal to be prompted for each value.`,
	Example: `  # From a repo
  usectl machines pods create api web --repo https://github.com/me/api --port 8080

  # From a prebuilt public image — no GitHub at all
  usectl machines pods create api cache --image redis:7 --port 6379 --private

  # From a private registry
  usectl machines pods create api web --image ghcr.io/acme/api:v1.2.3 --port 8080 \
    --registry-user acme-bot --registry-password "$GHCR_TOKEN"

  # Attach addons as part of creating the pod
  usectl machines pods create api web --repo https://github.com/me/api \
    --port 8080 --addon database --addon redis

  # Worker pod: no Service, no domain, runs a command
  usectl machines pods create api worker --repo https://github.com/me/api \
    --kind worker --command "node worker.js"

  # Interactive
  usectl machines pods create api`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID, err := resolveMachine(client, args[0])
		if err != nil {
			return err
		}

		name := podCreateName
		if len(args) == 2 {
			name = args[1]
		}

		if interactive() {
			fmt.Println("Add a pod:")
			if name == "" {
				name = ask("Name", "")
			}
			// Source is the one genuinely branching choice, so ask it before
			// anything that depends on it.
			if podCreateRepo == "" && podCreateImage == "" {
				switch strings.ToLower(ask("Source (repo/image)", "repo")) {
				case "image", "i":
					podCreateImage = ask("Image ref", "")
					if u := ask("Registry user (blank = public)", ""); u != "" {
						podCreateRegUser = u
						podCreateRegPass = ask("Registry password/token", "")
					}
				default:
					podCreateRepo = ask("Repo URL", "")
					podCreateBranch = ask("Branch", podCreateBranch)
				}
			}
			if !cmd.Flags().Changed("kind") {
				podCreateKind = ask("Kind (web/worker/release)", podCreateKind)
			}
			if podCreateKind == "web" && !cmd.Flags().Changed("port") {
				if p, pErr := strconv.Atoi(ask("Port", strconv.Itoa(podCreatePort))); pErr == nil {
					podCreatePort = p
				}
			}
			// A public web pod with no domain has no IngressRoute, so nothing
			// can reach it — the pod runs and is simply unreachable. The
			// prompt therefore defaults to a working subdomain instead of
			// offering "none": pressing enter must produce something usable.
			//
			// The deployer appends .usectl.com to any value without a dot
			// (deployer/k8s.go), so a bare label is a platform subdomain and a
			// dotted value is the user's own domain.
			if podCreateKind == "web" && !podCreatePrivate && podCreateDomain == "" {
				fmt.Println(output.Dim("  a bare name becomes <name>.usectl.com; enter a dotted name to use your own domain"))
				fmt.Println(output.Dim("  enter '-' for no domain (reachable only from other pods in the machine)"))
				podCreateDomain = ask("Domain", name)
				if podCreateDomain == "-" || strings.EqualFold(podCreateDomain, "none") {
					podCreateDomain = ""
				}
			}
			if podCreateKind != "web" && podCreateCommand == "" {
				podCreateCommand = ask("Command", "")
			}
			if len(podCreateArgs) == 0 && podCreateCommand != "" {
				// Split on spaces, honouring quotes, so an argument containing
				// a space survives: --arg is repeatable but a prompt is one line.
				if raw := ask("Args (space separated, blank = none)", ""); raw != "" {
					parsed, aErr := splitArgs(raw)
					if aErr != nil {
						return aErr
					}
					podCreateArgs = parsed
				}
			}
			// Offer the machine's addons rather than making the user go and
			// look them up.
			if len(podCreateAddons) == 0 && !podCreateAllAddons {
				if existing, aErr := client.ListProjectAddons(machineID); aErr == nil && len(existing) > 0 {
					labels := make([]string, len(existing))
					for i, a := range existing {
						labels[i] = a.AddonType + "/" + a.Name
					}
					fmt.Printf("  available addons: %s\n", strings.Join(labels, ", "))
					if picked := ask("Attach addons (comma-separated, blank = none)", ""); picked != "" {
						for _, p := range strings.Split(picked, ",") {
							if p = strings.TrimSpace(p); p != "" {
								podCreateAddons = append(podCreateAddons, p)
							}
						}
					}
				}
			}
		}

		var missing []string
		if name == "" {
			missing = append(missing, "name")
		}
		if podCreateRepo == "" && podCreateImage == "" {
			missing = append(missing, "repo or image")
		}
		if err := requireInteractive(missing,
			"usectl machines pods create <machine> <name> (--repo <url> | --image <ref>) [--port N]"); err != nil {
			return err
		}

		// Mirrors the server's rule: an app carrying both a repo and an image
		// has no single answer to what a deploy builds.
		if podCreateRepo != "" && podCreateImage != "" {
			return fmt.Errorf("--repo and --image are mutually exclusive: a pod is built from a repo OR deployed from a prebuilt image")
		}
		if podCreateImage == "" && (podCreateRegUser != "" || podCreateRegPass != "") {
			return fmt.Errorf("--registry-user/--registry-password only apply with --image")
		}

		extra, err := parseExtraPorts(podCreateExtraPorts)
		if err != nil {
			return err
		}

		// Resolve addon references up front: failing here costs nothing,
		// whereas failing after the pod exists leaves it half-configured.
		addonIDs := make([]string, 0, len(podCreateAddons))
		if podCreateAllAddons {
			all, aErr := client.ListProjectAddons(machineID)
			if aErr != nil {
				return aErr
			}
			for _, a := range all {
				addonIDs = append(addonIDs, a.ID)
			}
		}
		for _, ref := range podCreateAddons {
			id, rErr := resolveAddon(client, machineID, ref)
			if rErr != nil {
				return rErr
			}
			addonIDs = append(addonIDs, id)
		}

		sourceType := ""
		if podCreateImage != "" {
			sourceType = "image"
		}
		isPublic := !podCreatePrivate

		req := api.CreateProjectAppRequest{
			Name:             name,
			SourceType:       sourceType,
			ImageRef:         podCreateImage,
			RegistryUsername: podCreateRegUser,
			RegistryPassword: podCreateRegPass,
			RepoURL:          podCreateRepo,
			Branch:           podCreateBranch,
			Domain:           podCreateDomain,
			Port:             podCreatePort,
			Replicas:         podCreateReplicas,
			AutoDeploy:       podCreateAutoDeploy,
			IsPublic:         &isPublic,
			Kind:             podCreateKind,
			Command:          podCreateCommand,
			Args:             podCreateArgs,
			ExtraPorts:       extra,
		}
		if podCreateInstallID > 0 {
			req.InstallationID = &podCreateInstallID
		}

		if interactive() {
			fmt.Printf("\n  %-12s %s\n", "Name", name)
			if podCreateImage != "" {
				fmt.Printf("  %-12s image  %s\n", "Source", podCreateImage)
			} else {
				fmt.Printf("  %-12s git    %s @ %s\n", "Source", podCreateRepo, podCreateBranch)
			}
			fmt.Printf("  %-12s %s\n", "Kind", podCreateKind)
			if podCreateKind == "web" {
				fmt.Printf("  %-12s %d (%s)\n", "Port", podCreatePort,
					map[bool]string{true: "public", false: "internal"}[isPublic])
			}
			if podCreateKind == "web" {
				switch {
				case podCreatePrivate:
					fmt.Printf("  %-12s %s\n", "Reachable", output.Yellow("internal only (no IngressRoute)"))
				case podCreateDomain == "":
					fmt.Printf("  %-12s %s\n", "Reachable", output.Yellow("internal only — no domain, so nothing external can reach it"))
				default:
					fmt.Printf("  %-12s %s\n", "URL", output.Cyan("https://"+expandDomain(podCreateDomain)))
				}
			}
			if podCreateCommand != "" {
				fmt.Printf("  %-12s %s %s\n", "Command", podCreateCommand, strings.Join(podCreateArgs, " "))
			}
			if len(addonIDs) > 0 {
				fmt.Printf("  %-12s %d attached\n", "Addons", len(addonIDs))
			}
			fmt.Println()
			if !confirm("Create this pod?") {
				return fmt.Errorf("cancelled")
			}
		}

		app, err := client.CreateProjectApp(machineID, req)
		if err != nil {
			return err
		}

		// Attach after creation: attachment is a separate resource, so a
		// failure here leaves a usable pod rather than rolling back the create.
		var attachErrs []string
		for _, id := range addonIDs {
			if aErr := client.AttachAppAddon(machineID, app.ID, id); aErr != nil {
				attachErrs = append(attachErrs, fmt.Sprintf("%s: %v", id, aErr))
			}
		}

		if jsonOutput {
			return output.JSON(app)
		}
		fmt.Printf("✓ Pod created: %s (%s)\n", app.Name, app.ID)
		if podCreateKind == "web" {
			if podCreateDomain != "" {
				fmt.Printf("  URL        https://%s\n", expandDomain(podCreateDomain))
			} else if !podCreatePrivate {
				fmt.Println("  ⚠ No domain attached — this pod will not be reachable from outside the cluster.")
				fmt.Println("    Attach one:  usectl machines domains ...")
			}
		}
		if podCreateImage != "" {
			fmt.Printf("  Source     image %s (no build runs)\n", podCreateImage)
		} else {
			fmt.Printf("  Source     git %s @ %s\n", podCreateRepo, podCreateBranch)
		}
		if n := len(addonIDs) - len(attachErrs); n > 0 {
			fmt.Printf("  Addons     %d attached — credentials injected as env vars\n", n)
		}
		for _, e := range attachErrs {
			fmt.Printf("  ⚠ addon attach failed — %s\n", e)
		}
		fmt.Printf("\nDeploy it:\n  usectl machines deploy %s\n", args[0])
		return nil
	},
}

func init() {
	f := podsCreateCmd.Flags()
	f.StringVar(&podCreateName, "name", "", "Pod name (or pass it positionally)")
	f.StringVar(&podCreateRepo, "repo", "", "Git repository URL (mutually exclusive with --image)")
	f.StringVar(&podCreateImage, "image", "", "Prebuilt image reference, e.g. ghcr.io/acme/api:v1.2.3 — no GitHub, no build")
	f.StringVar(&podCreateRegUser, "registry-user", "", "Private registry username (with --image)")
	f.StringVar(&podCreateRegPass, "registry-password", "", "Private registry password or token (with --image)")
	f.StringVar(&podCreateBranch, "branch", "main", "Git branch (with --repo)")
	f.StringVar(&podCreateDomain, "domain", "", "Domain to attach to this pod")
	f.IntVar(&podCreatePort, "port", 3000, "Container port")
	f.IntVar(&podCreateReplicas, "replicas", 1, "Replica count")
	f.BoolVar(&podCreatePrivate, "private", false, "Cluster-internal only — no IngressRoute")
	f.StringVar(&podCreateKind, "kind", "web", "Pod kind: web, worker, release")
	f.StringVar(&podCreateCommand, "command", "", "Override the container command (required for worker/release)")
	f.StringArrayVar(&podCreateArgs, "arg", nil, "Container argument, after --command (repeatable)")
	f.StringArrayVar(&podCreateExtraPorts, "extra-port", nil, "Extra internal-only port [name:]port[/proto] (repeatable)")
	f.StringSliceVar(&podCreateAddons, "addon", nil, "Attach an addon by name, type, or type/name (repeatable)")
	f.BoolVar(&podCreateAllAddons, "all-addons", false, "Attach every addon in the machine")
	f.BoolVar(&podCreateAutoDeploy, "auto-deploy", false, "Deploy automatically on push (repo source only)")
	f.Int64Var(&podCreateInstallID, "installation-id", 0, "GitHub App installation ID (repo source only)")
	f.BoolVarP(&assumeYes, "yes", "y", false, "Do not prompt; fail instead if a required value is missing")
	podsDeleteCmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "Skip the delete confirmation")
}

var podsDeleteCmd = &cobra.Command{
	Use:     "delete [machine] <pod>",
	Aliases: []string{"rm", "remove"},
	Short:   "Delete a pod and its Kubernetes resources",
	Long: `Remove a pod from a machine. Its Deployment, Service and IngressRoute are
torn down; the machine, its addons and its other pods are untouched.

Irreversible, so the pod's name must be typed to confirm — or --yes passed.`,
	Example: `  usectl machines pods delete api worker
  usectl machines pods delete api worker --yes`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID, podID, _, err := resolveMachineAndPod(client, args)
		if err != nil {
			return err
		}

		// Resolve the real name for the prompt: the user may have typed a
		// UUID or a prefix, and the confirmation should name what actually
		// goes away.
		name := podID
		if apps, aErr := client.ListProjectApps(machineID); aErr == nil {
			for _, a := range apps {
				if a.ID == podID {
					name = a.Name
				}
			}
		}

		if !assumeYes {
			if !interactive() {
				return fmt.Errorf("refusing to delete pod %q without confirmation — pass --yes to proceed non-interactively", name)
			}
			fmt.Printf("This deletes pod %q and its Deployment, Service and IngressRoute.\n", name)
			fmt.Println("  Addons and other pods in the machine are unaffected. This cannot be undone.")
			if ans := ask("Type the pod name to confirm", ""); ans != name {
				return fmt.Errorf("cancelled — %q does not match %q", ans, name)
			}
		}

		if err := client.DeleteProjectApp(machineID, podID); err != nil {
			return err
		}
		fmt.Printf("✓ Pod %q deleted\n", name)
		return nil
	},
}

// expandDomain mirrors the deployer: a value with no dot is a platform
// subdomain, anything else is used verbatim.
func expandDomain(d string) string {
	if d == "" || strings.Contains(d, ".") {
		return d
	}
	return d + ".usectl.com"
}
