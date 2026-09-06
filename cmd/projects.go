package cmd

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/config"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

// machinesCmd is the renamed top-level command for what the API still calls
// "projects". The dashboard switched to "Machine" terminology in
// PROMPT_rename_project_to_machine_minimal.md; the CLI follows here.
// `projects` / `project` / `p` stay as aliases so existing scripts and muscle
// memory keep working.
var projectsCmd = &cobra.Command{
	Use:     "machines",
	Aliases: []string{"machine", "m", "projects", "project", "p"},
	Short:   "Manage machines — create, resize, deploy, monitor",
	Long: `A MACHINE is a Kubernetes namespace plus a resource wallet: vCPU, RAM,
storage and one billing subscription. It holds no code of its own.

A POD is one workload inside a machine. Repository and branch (or a prebuilt
image), domain, ports, visibility, container limits and rollout strategy are
all per-pod — a machine with three pods has three repos and three sets of
ports.

  usectl machines create api --vcpu 2 --ram 4 --storage 10
  usectl machines pods create api web --repo https://github.com/me/api --port 8080
  usectl machines pods api

Addons are provisioned per machine and must be ATTACHED to a pod before that
pod receives their credentials as environment variables.

Machines, pods and addons can all be referred to by name. Set a default with
'usectl use <machine>' (or -m / $USECTL_MACHINE) and the machine argument
becomes optional everywhere.

Where a command takes both a machine and a pod (or addon), the order does not
matter — machine and pod names resolve against different collections, so only
one reading of the pair can be valid.

Subcommands:
  list / get / create / delete   machine lifecycle
  pods                           every pod: config, limits, node, addons
  addons                         provision and inspect addons
  usage / quota                  consumption against the plan
  settings                       machine-level configuration
  members / groups               access control and namespace isolation
  deploy / deployments / rollback / logs / shell / diagnostics
  enter                          interactive shell scoped to one machine`,
}

var projectsListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all projects with their status, domain, and features",
	Long: `Returns a table of all projects the authenticated user has access to.
Admin users see all projects. Columns include ID, name, domain, type,
latest deployment status, enabled features (db/s3), and branch.

Use --json for structured output suitable for scripting or AI agents.`,
	Example: `  usectl machines list
  usectl machines list --json
  usectl m ls`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		projects, err := client.ListProjects()
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(projects)
		}

		// DOMAIN and BRANCH deliberately absent: both are per-pod settings.
		// A machine with three pods has three branches and up to three
		// domains, so a single column here could only ever be misleading.
		// `usectl machines pods <machine>` is where they belong.
		rows := make([][]string, len(projects))
		for i, pw := range projects {
			p := pw.Project
			status := "-"
			if pw.LatestDeployment != nil {
				status = pw.LatestDeployment.Status
			}
			rows[i] = []string{
				output.Dim(p.ID),
				output.Bold(p.Name),
				deployStatusColored(status),
				trimFloat(p.VCPU),
				trimFloat(p.RAMGB) + "G",
				trimFloat(p.StorageGB) + "G",
				billingSummary(p),
			}
		}
		output.Table([]string{"ID", "NAME", "STATUS", "VCPU", "RAM", "STORAGE", "BILLING"}, rows)
		return nil
	},
}

var projectsGetCmd = &cobra.Command{
	Use:   "get [machine]",
	Short: "Get detailed project information including database and S3 status",
	Long: `Returns detailed information about a single project, including repo URL,
branch, domain, port, database provisioning status, S3 bucket status,
creation date, and recent deployment history.

The <id> can be the full UUID or a prefix (e.g. first 8 chars).`,
	Example: `  usectl machines get a8f15889
  usectl machines get a8f15889-3636-402d-99a1-3492ba6b4383 --json`,
	Args: cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}
		resp, err := client.GetProjectFull(args[0])
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(resp)
		}

		project := resp.Project
		dbStatus := "no"
		if project.NeedsDB {
			dbStatus = "yes"
			if project.DBName != nil {
				dbStatus = fmt.Sprintf("yes (%s)", *project.DBName)
			}
		}
		s3Status := "no"
		if project.NeedsS3 {
			s3Status = "yes"
			if project.S3Bucket != nil {
				s3Status = fmt.Sprintf("yes (%s)", *project.S3Bucket)
			}
		}

		previewEnvs := "no"
		if project.EnablePreviewEnvs {
			previewEnvs = "yes"
		}

		displayDomain := project.Domain + ".usectl.com"
		if strings.Contains(project.Domain, ".") {
			displayDomain = project.Domain
		}
		fields := [][]string{
			{"ID", project.ID},
			{"Name", project.Name},
			{"Namespace", "kdeploy-" + project.Name},
			{"Created", project.CreatedAt},
			{"", ""},
			{"vCPU", trimFloat(project.VCPU)},
			{"RAM", trimFloat(project.RAMGB) + " GB"},
			{"Storage", trimFloat(project.StorageGB) + " GB"},
			{"", ""},
			{"Billing", project.BillingStatus},
			{"Interval", project.BillingInterval},
			{"Price", fmt.Sprintf("$%.2f / %s", float64(project.MonthlyPriceCents)/100, orDash(project.BillingInterval))},
		}
		if project.TrialEndsAt != nil {
			fields = append(fields, []string{"Trial ends", *project.TrialEndsAt})
		}
		fields = append(fields,
			[]string{"", ""},
			[]string{"Database", dbStatus},
			[]string{"Object Storage", s3Status},
			[]string{"Preview Envs", previewEnvs},
		)
		// Legacy single-app machines still carry these; newer ones keep repo,
		// branch, domain and port on each pod instead, so only show them when
		// the machine actually has them set.
		if project.RepoURL != "" {
			fields = append(fields,
				[]string{"", ""},
				[]string{"Repo (legacy)", project.RepoURL},
				[]string{"Branch (legacy)", project.Branch},
				[]string{"Domain (legacy)", displayDomain},
				[]string{"Port (legacy)", strconv.Itoa(project.Port)},
			)
		}
		output.Table([]string{"FIELD", "VALUE"}, fields)

		// Recent deployments come from the paginated endpoint, not the slice
		// embedded in the machine object: that field is scoped to the legacy
		// top-level app, so on a per-pod machine it is empty and the section
		// silently vanished.
		if page, dErr := client.ListDeployments(args[0], "", "", 1, 5); dErr == nil && page != nil && len(page.Deployments) > 0 {
			podName := map[string]string{}
			if apps, aErr := client.ListProjectApps(args[0]); aErr == nil {
				for _, a := range apps {
					podName[a.ID] = a.Name
				}
			}
			fmt.Println()
			fmt.Println(output.Bold("Recent deployments"))
			rows := make([][]string, 0, len(page.Deployments))
			for _, d := range page.Deployments {
				commit := d.CommitHash
				if len(commit) > 7 {
					commit = commit[:7]
				}
				pod := "—"
				if d.ProjectAppID != nil {
					if n, ok := podName[*d.ProjectAppID]; ok {
						pod = n
					}
				}
				rows = append(rows, []string{output.Dim(shortID(d.ID)), pod, deployStatusColored(d.Status), commit, d.CreatedAt})
			}
			output.Table([]string{"ID", "POD", "STATUS", "COMMIT", "CREATED"}, rows)
			if page.Total > len(page.Deployments) {
				fmt.Printf("%s\n", output.Dim(fmt.Sprintf("  … %d more — usectl machines deployments %s", page.Total-len(page.Deployments), args[0])))
			}
		}
		return nil
	},
}

// Flags for create command.
//
// A machine is a namespace + a resource wallet + a billing subscription. What
// used to live here — repo, branch, domain, port, type — is per-POD config and
// now belongs to `usectl machines pods create`. The server agrees: CreateProject
// requires only a name, and stopped auto-generating a <name>.usectl.com domain.
var (
	createName      string
	createVCPU      float64
	createRAM       string
	createStorage   string
	createBilling   string
	createDB        bool
	createS3        bool
	createGHToken   string
	createInstallID int64
	createAddons    []string
	createEnvs      []string
)

var projectsCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a new machine (a namespace + resource quota)",
	Long: `Create a machine: a Kubernetes namespace (kdeploy-<name>), a resource quota
(vCPU / RAM / storage), and a billing subscription.

A machine holds no code of its own. Repositories, domains, ports and container
sizing are per-POD settings — create the machine first, then add pods to it:

  usectl machines create api --vcpu 2 --ram 4 --storage 10
  usectl machines pods create api --repo https://github.com/me/api --port 8080

Run with no flags on a terminal to be prompted for each value.

Sizes accept a bare number of GB or an explicit suffix: --ram 4, --ram 4gb and
--ram 4096mb are the same machine. The chosen size is always echoed back in GB
before anything is created.

Machine names must be unique across the platform, because the namespace is
derived from the name — "Test CLI" and "test-cli" would resolve to the same
namespace.`,
	Example: `  # Interactive — prompts for name, vCPU, RAM, storage
  usectl machines create

  # Fully specified
  usectl machines create test-cli --vcpu 10 --ram 1024mb --storage 2

  # With addons provisioned up front
  usectl machines create api --vcpu 2 --ram 4 --storage 20 --db --s3

  # Machine-wide environment variables
  usectl machines create api --vcpu 1 --ram 2 --storage 5 \
    --env NODE_ENV=production --env LOG_LEVEL=debug`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}

		name := createName
		if len(args) == 1 {
			name = args[0]
		}

		ramGB, err := sizeFlag(cmd, "ram", createRAM)
		if err != nil {
			return err
		}
		storageGB, err := sizeFlag(cmd, "storage", createStorage)
		if err != nil {
			return err
		}

		// Prompt only for what is actually missing, and only when prompting is
		// permitted. --json / --yes / a piped stdin take the error path below
		// instead of blocking on a read no caller can see.
		if interactive() {
			fmt.Println("Create a machine:")
			if name == "" {
				name = ask("Name", "")
			}
			if !cmd.Flags().Changed("vcpu") {
				createVCPU = askFloat("vCPU", createVCPU)
			}
			if !cmd.Flags().Changed("ram") {
				ramGB = askSize("RAM (GB)", ramGB)
			}
			if !cmd.Flags().Changed("storage") {
				storageGB = askSize("Storage (GB)", storageGB)
			}
			if !cmd.Flags().Changed("billing") {
				createBilling = ask("Billing", createBilling)
			}
		}

		var missing []string
		if name == "" {
			missing = append(missing, "name")
		}
		if err := requireInteractive(missing,
			"usectl machines create <name> [--vcpu N] [--ram N] [--storage N]"); err != nil {
			return err
		}
		if createBilling != "month" && createBilling != "year" {
			return fmt.Errorf("--billing must be 'month' or 'year', got %q", createBilling)
		}

		allAddons := make(map[string]bool)
		for _, a := range createAddons {
			allAddons[a] = true
		}
		if createDB {
			allAddons["database"] = true
		}
		if createS3 {
			allAddons["s3"] = true
		}
		addonsList := make([]string, 0, len(allAddons))
		for a := range allAddons {
			addonsList = append(addonsList, a)
		}
		sort.Strings(addonsList)

		// Auto-detect the GitHub App installation so pods created later in this
		// machine can clone private repos without re-supplying it.
		if createInstallID == 0 {
			cfg, _ := config.Load()
			if cfg != nil && cfg.GitHubToken != "" {
				if installations, iErr := client.ListGitHubInstallations(cfg.GitHubToken); iErr == nil && len(installations) > 0 {
					createInstallID = installations[0].ID
				}
			}
		}

		req := api.CreateProjectRequest{
			Name:            name,
			VCPU:            createVCPU,
			RAMGB:           ramGB,
			StorageGB:       storageGB,
			BillingInterval: createBilling,
			NeedsDB:         allAddons["database"],
			NeedsS3:         allAddons["s3"],
			GithubToken:     createGHToken,
			Addons:          addonsList,
		}
		if createInstallID > 0 {
			req.InstallationID = &createInstallID
		}

		if interactive() {
			fmt.Printf("\n  %-14s  %s\n", "Name", name)
			fmt.Printf("  %-14s  %s vCPU · %s GB RAM · %s GB storage\n", "Size",
				trimFloat(createVCPU), trimFloat(ramGB), trimFloat(storageGB))
			fmt.Printf("  %-14s  %sly\n", "Billing", createBilling)
			if len(addonsList) > 0 {
				fmt.Printf("  %-14s  %s\n", "Addons", strings.Join(addonsList, ", "))
			}
			fmt.Println()
			if !confirm("Create this machine?") {
				return fmt.Errorf("cancelled")
			}
		}

		project, err := client.CreateProject(req)
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(project)
		}

		fmt.Printf("✓ Machine created: %s (%s)\n", project.Name, project.ID)
		fmt.Printf("  Namespace  kdeploy-%s\n", project.Name)
		fmt.Printf("  Size       %s vCPU · %s GB RAM · %s GB storage\n",
			trimFloat(createVCPU), trimFloat(ramGB), trimFloat(storageGB))

		if len(createEnvs) > 0 {
			vars := make(map[string]string)
			for _, e := range createEnvs {
				if parts := strings.SplitN(e, "=", 2); len(parts) == 2 {
					vars[parts[0]] = parts[1]
				}
			}
			if len(vars) > 0 {
				if err := client.SetEnvs(project.ID, vars); err != nil {
					fmt.Printf("  ⚠ failed to set env vars: %v\n", err)
				} else {
					fmt.Printf("  Env        %d variable(s) set\n", len(vars))
				}
			}
		}

		fmt.Printf("\nNext: add a pod\n  usectl machines pods create %s --repo <url> --port <port>\n", project.Name)
		return nil
	},
}

// sizeFlag resolves a --ram / --storage string flag into GB. An unset flag
// keeps its default without going through the parser, so a malformed default
// can never fail a command that did not use the flag.
func sizeFlag(cmd *cobra.Command, name, raw string) (float64, error) {
	v, err := parseSizeGB(raw)
	if err != nil {
		return 0, fmt.Errorf("--%s: %w", name, err)
	}
	return v, nil
}

// Flags for update command
var (
	updateName        string
	updateDomain      string
	updateBranch      string
	updatePort        int
	updateGHToken     string
	updateInstallID   int64
	updatePreviewEnvs bool
)

var projectsUpdateCmd = &cobra.Command{
	Use:   "update [machine]",
	Short: "Update project settings (domain, branch, port, etc.)",
	Long: `Modify one or more settings of an existing project. Only the flags you
provide will be updated — omitted fields remain unchanged.

If --port or --domain is changed, the K8s resources (Deployment, Service,
IngressRoute) are automatically updated in the background.`,
	Example: `  usectl machines update a8f15889 --port 3000
  usectl machines update a8f15889 --domain new-domain --branch develop
  usectl machines update a8f15889 --installation-id 114078944
  usectl machines update a8f15889 --preview-envs       # Enable PR preview environments
  usectl machines update a8f15889 --preview-envs=false  # Disable PR preview environments`,
	Args: cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}

		req := api.UpdateProjectRequest{}
		if cmd.Flags().Changed("name") {
			req.Name = &updateName
		}
		if cmd.Flags().Changed("domain") {
			req.Domain = &updateDomain
		}
		if cmd.Flags().Changed("branch") {
			req.Branch = &updateBranch
		}
		if cmd.Flags().Changed("port") {
			req.Port = &updatePort
		}
		if cmd.Flags().Changed("github-token") {
			req.GithubToken = &updateGHToken
		}
		if cmd.Flags().Changed("installation-id") {
			req.InstallationID = &updateInstallID
		}
		if cmd.Flags().Changed("preview-envs") {
			req.EnablePreviewEnvs = &updatePreviewEnvs
		}

		project, err := client.UpdateProject(args[0], req)
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(project)
		}

		fmt.Printf("✓ Project updated: %s\n", project.Name)
		return nil
	},
}

var projectsDeleteCmd = &cobra.Command{
	Use:   "delete <machine>",
	Short: "Delete a project and all associated resources (namespace, DB, S3)",
	Long: `Permanently delete a project and clean up all associated resources:
  - Kubernetes namespace and all resources inside (pods, services, ingress)
  - Provisioned PostgreSQL database and user (if --db was used)
  - S3 bucket, objects, user, and policy (if --s3 was used)

This action is irreversible.`,
	Example: `  usectl machines delete my-machine
  usectl machines delete my-machine --yes    # skip the confirmation`,
	// Deliberately ExactArgs(1) rather than the optional-machine form the other
	// commands use: this is irreversible, and a machine left over in
	// `usectl use` must never be enough to destroy it by typing four words.
	// The target has to be named.
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID, err := resolveMachine(client, args[0])
		if err != nil {
			return err
		}
		// Resolve the name for the prompt so the confirmation names what will
		// actually be destroyed, not the abbreviation that was typed.
		name := args[0]
		if resp, gErr := client.GetProjectFull(machineID); gErr == nil {
			name = resp.Project.Name
		}

		if !assumeYes {
			if !interactive() {
				return fmt.Errorf("refusing to delete %q without confirmation — pass --yes to proceed non-interactively", name)
			}
			fmt.Printf("This permanently deletes machine %q (%s):\n", name, machineID)
			fmt.Println("  · the Kubernetes namespace and everything in it")
			fmt.Println("  · provisioned databases and their data")
			fmt.Println("  · S3 buckets and their objects")
			fmt.Println("  This cannot be undone.")
			if ans := ask("Type the machine name to confirm", ""); ans != name {
				return fmt.Errorf("cancelled — %q does not match %q", ans, name)
			}
		}

		if err := client.DeleteProject(machineID); err != nil {
			return err
		}
		fmt.Printf("✓ Machine %q deleted\n", name)
		return nil
	},
}

var projectsDeployCmd = &cobra.Command{
	Use:   "deploy [machine] [pod]",
	Short: "Build and deploy the latest commit",
	Long: `Trigger a build and deployment.

Naming a pod deploys just that workload. With no pod, every git-sourced pod in
the machine is deployed, one request each.

That per-pod fan-out is deliberate: repository and branch live on the pod, not
the machine, so there is no single HEAD for a machine to resolve. Asking the
server for a whole-machine deploy fails with "commit_hash is required (could
not auto-resolve HEAD)" on any machine that does not carry a legacy top-level
repo.

Image-sourced pods are skipped — they have no repo to build from and are
redeployed by changing their image reference.`,
	Example: `  usectl machines deploy api          # every git-sourced pod
  usectl machines deploy api web      # just this pod
  usectl machines deploy web api      # same — the order does not matter`,
	Args: cobra.RangeArgs(0, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID, podID, err := resolveMachineOptionalPod(client, args)
		if err != nil {
			return err
		}

		apps, err := client.ListProjectApps(machineID)
		if err != nil {
			return err
		}

		targets := make([]api.ProjectApp, 0, len(apps))
		for _, a := range apps {
			if podID != "" {
				if a.ID == podID {
					targets = append(targets, a)
				}
				continue
			}
			if a.SourceType == "image" || (a.RepoURL == "" && a.ImageRef != "") {
				continue // nothing to build
			}
			targets = append(targets, a)
		}

		if len(targets) == 0 {
			if podID != "" {
				return fmt.Errorf("pod not found in this machine")
			}
			return fmt.Errorf("no git-sourced pods to deploy in this machine")
		}

		var failures int
		for _, a := range targets {
			resp, dErr := client.DeployProject(machineID, a.ID)
			if dErr != nil {
				failures++
				fmt.Printf("  %s %s: %v\n", output.Red("✗"), a.Name, dErr)
				continue
			}
			id := ""
			if resp != nil {
				id = resp.Deployment.ID
			}
			fmt.Printf("  %s %s %s\n", output.Green("✓"), a.Name, output.Dim(id))
		}
		if failures > 0 {
			return fmt.Errorf("%d of %d pod(s) failed to start deploying", failures, len(targets))
		}
		fmt.Printf("\nWatch progress:\n  usectl machines deployments %s\n", firstOrEmpty(args))
		return nil
	},
}

// Flags for logs
var (
	logsFollow bool
	logsLines  int
)

var projectsLogsCmd = &cobra.Command{
	Use:   "logs [machine] [pod]",
	Short: "View live runtime logs from the running application containers",
	Long: `Fetch the latest log output from the machine's running pods.
Use --tail to control how many lines to retrieve (default: 100).
Use -f / --follow to stream logs in real-time (like docker logs -f).`,
	Example: `  usectl machines logs api
  usectl machines logs api web            # only this pod
  usectl machines logs api web -f
  usectl machines logs api --tail 500`,
	Args: cobra.RangeArgs(0, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		// Naming a pod narrows the stream. Without it a multi-pod machine
		// interleaves output from unrelated workloads, which is rarely what
		// someone tailing logs wants.
		machineID, podID, err := resolveMachineOptionalPod(client, args)
		if err != nil {
			return err
		}

		// Follow mode: stream to stdout.
		if logsFollow {
			return client.StreamRuntimeLogs(machineID, logsLines, podID, os.Stdout)
		}

		// Normal mode: fetch and print.
		logs, err := client.GetRuntimeLogs(machineID, logsLines, podID)
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(logs)
		}

		fmt.Print(logs.Logs)
		return nil
	},
}

var projectsBuildLogsCmd = &cobra.Command{
	Use:   "build-logs [machine] <deployment-id>",
	Short: "View build and deploy logs for a specific deployment",
	Long: `Retrieve the full build log (clone + Kaniko build) and deploy log for a
specific deployment. Use 'usectl machines get <id>' to see deployment IDs.`,
	Example: `  usectl machines build-logs a8f15889 d4e5f6a7`,
	Args:    cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}
		if len(args) < 2 {
			return fmt.Errorf("a deployment id is required — run 'usectl machines deployments %s'", args[0])
		}
		// Accept the shortened id the listings print, not just the full UUID.
		if args[1], err = resolveDeployment(client, args[0], args[1]); err != nil {
			return err
		}
		logs, err := client.GetDeploymentLogs(args[0], args[1])
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(logs)
		}

		if logs.Log != "" {
			fmt.Println(logs.Log)
		} else {
			fmt.Println("(no logs available)")
		}
		return nil
	},
}

var (
	depsStatus  string
	depsPage    int
	depsPerPage int
)

var projectsDeploymentsCmd = &cobra.Command{
	Use:     "deployments [machine] [pod]",
	Aliases: []string{"deps"},
	Short:   "List deployment history, newest first",
	Long: `Deployment history for a machine, or for one pod.

Reads GET /projects/{id}/deployments, which paginates and filters server-side.
It previously read the deployment list embedded in the machine object instead —
that field carries only a small recent slice scoped to the legacy top-level
app, so a machine whose pods each have their own repo showed "No deployments
found" while the dashboard listed plenty.`,
	Example: `  usectl machines deployments api
  usectl machines deployments api web
  usectl machines deployments api --status failed --per-page 50`,
	Args: cobra.RangeArgs(0, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID, podID, err := resolveMachineOptionalPod(client, args)
		if err != nil {
			return err
		}

		page, err := client.ListDeployments(machineID, depsStatus, podID, depsPage, depsPerPage)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(page)
		}
		if page == nil || len(page.Deployments) == 0 {
			fmt.Println("No deployments found.")
			return nil
		}

		// Pod names are resolved once so each row can say which workload it
		// belongs to — on a multi-pod machine an undifferentiated list of
		// commits is close to unreadable.
		podName := map[string]string{}
		if apps, aErr := client.ListProjectApps(machineID); aErr == nil {
			for _, a := range apps {
				podName[a.ID] = a.Name
			}
		}

		rows := make([][]string, len(page.Deployments))
		for i, d := range page.Deployments {
			commit := d.CommitHash
			if len(commit) > 7 {
				commit = commit[:7]
			}
			pod := "—"
			if d.ProjectAppID != nil {
				if n, ok := podName[*d.ProjectAppID]; ok {
					pod = n
				} else {
					pod = shortID(*d.ProjectAppID)
				}
			}
			rows[i] = []string{
				output.Dim(shortID(d.ID)),
				pod,
				deployStatusColored(d.Status),
				commit,
				d.CreatedAt,
			}
		}
		output.Table([]string{"ID", "POD", "STATUS", "COMMIT", "CREATED"}, rows)
		fmt.Printf("\n%s\n", output.Dim(fmt.Sprintf("page %d of %d · %d deployment(s) total",
			page.Page, page.TotalPages, page.Total)))
		return nil
	},
}

var projectsRollbackCmd = &cobra.Command{
	Use:   "rollback [machine] <deployment-id>",
	Short: "Roll back to a previous deployment (redeploy its container image)",
	Long: `Roll back a project to a previously successful deployment by redeploying
its container image without rebuilding. This is useful to quickly recover from
a bad deployment.

The deployment-id should reference an existing deployment. Use
'usectl machines deployments [machine]' to see available deployments.`,
	Example: `  usectl machines rollback a8f15889 d4e5f6a7`,
	Args:    cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}
		if len(args) < 2 {
			return fmt.Errorf("a deployment id is required — run 'usectl machines deployments %s'", args[0])
		}
		// Accept the shortened id the listings print, not just the full UUID.
		if args[1], err = resolveDeployment(client, args[0], args[1]); err != nil {
			return err
		}

		// Fetch the full project to find the target deployment's commit hash.
		resp, err := client.GetProjectFull(args[0])
		if err != nil {
			return err
		}

		var targetCommit string
		var targetStatus string
		for _, d := range resp.Deployments {
			if d.ID == args[1] || strings.HasPrefix(d.ID, args[1]) {
				targetCommit = d.CommitHash
				targetStatus = d.Status
				break
			}
		}
		if targetCommit == "" {
			return fmt.Errorf("deployment %s not found in project %s", args[1], args[0])
		}

		if targetStatus != "deployed" && targetStatus != "running" {
			fmt.Printf("⚠ Warning: target deployment status is '%s' (not 'deployed'). Proceeding anyway.\n", targetStatus)
		}

		shortCommit := targetCommit
		if len(shortCommit) > 7 {
			shortCommit = shortCommit[:7]
		}
		fmt.Printf("Rolling back to commit %s...\n", shortCommit)

		if err := client.RollbackProject(args[0], targetCommit); err != nil {
			return fmt.Errorf("rollback failed: %w", err)
		}

		fmt.Printf("✓ Rollback triggered. Redeploying image from commit %s (skip build).\n", shortCommit)
		fmt.Println("  Use 'usectl machines logs [machine]' to monitor.")
		return nil
	},
}

var projectsStartCmd = &cobra.Command{
	Use:     "start [machine]",
	Short:   "Start a stopped project (scale replicas to 1)",
	Example: `  usectl machines start a8f15889`,
	Args:    cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}
		if err := client.StartProject(args[0]); err != nil {
			return err
		}
		fmt.Println("✓ Project started")
		return nil
	},
}

var projectsStopCmd = &cobra.Command{
	Use:     "stop [machine]",
	Short:   "Stop a running project (scale replicas to 0)",
	Example: `  usectl machines stop a8f15889`,
	Args:    cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}
		if err := client.StopProject(args[0]); err != nil {
			return err
		}
		fmt.Println("✓ Project stopped")
		return nil
	},
}

var projectsStatusCmd = &cobra.Command{
	Use:     "status [machine]",
	Short:   "Check if the machine's containers are running or stopped",
	Long:    `Returns the running status and current replica count of the machine's deployment.`,
	Example: `  usectl machines status a8f15889`,
	Args:    cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}
		status, err := client.GetProjectStatus(args[0])
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(status)
		}

		fmt.Printf("Status: %s (replicas: %d)\n", status.Status, status.Replicas)
		return nil
	},
}

var projectsStatsCmd = &cobra.Command{
	Use:   "stats [machine]",
	Short: "View CPU, memory, network, database size, and storage usage",
	Long: `Returns resource usage metrics for the project, including:
  - Per-pod CPU and memory usage
  - Network RX/TX
  - Pod restart count
  - Database size (if provisioned)
  - S3 storage used (if provisioned)`,
	Example: `  usectl machines stats a8f15889
  usectl machines stats a8f15889 --json`,
	Args: cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}
		stats, err := client.GetProjectStats(args[0])
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(stats)
		}

		if stats.DBSize != "" {
			fmt.Printf("Database Size: %s\n", stats.DBSize)
		}
		if stats.StorageUsed != "" {
			fmt.Printf("Storage Used:  %s\n", stats.StorageUsed)
		}
		if len(stats.Pods) > 0 {
			rows := make([][]string, len(stats.Pods))
			for i, p := range stats.Pods {
				rows[i] = []string{p.Name, p.Status, p.CPU, p.Memory, p.NetRx, p.NetTx, strconv.Itoa(int(p.Restarts))}
			}
			output.Table([]string{"POD", "STATUS", "CPU", "MEMORY", "NET RX", "NET TX", "RESTARTS"}, rows)
		}
		return nil
	},
}

var projectsPRsCmd = &cobra.Command{
	Use:     "prs [machine]",
	Aliases: []string{"pr", "previews"},
	Short:   "List active PR preview deployments for a machine",
	Long: `Returns a table of active pull request preview environments for the given
project. Each PR preview has its own domain, namespace, and deployment.

Preview environments are automatically created when a PR is opened or updated,
and cleaned up when the PR is closed or merged.

Enable preview environments with:
  usectl machines update <id> --preview-envs`,
	Example: `  usectl machines prs a8f15889
  usectl machines prs a8f15889 --json`,
	Args: cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}
		prs, err := client.ListActivePRs(args[0])
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(prs)
		}

		if len(prs) == 0 {
			fmt.Println("No active PR preview environments.")
			fmt.Println("\nHint: Enable preview envs with 'usectl machines update <id> --preview-envs'")
			return nil
		}

		rows := make([][]string, len(prs))
		for i, d := range prs {
			prNum := "-"
			if d.PRNumber != nil {
				prNum = fmt.Sprintf("#%d", *d.PRNumber)
			}
			prBranch := "-"
			if d.PRBranch != nil {
				prBranch = *d.PRBranch
			}
			prDomain := "-"
			if d.PRDomain != nil {
				prDomain = *d.PRDomain
			}
			commit := d.CommitHash
			if len(commit) > 7 {
				commit = commit[:7]
			}
			rows[i] = []string{prNum, prBranch, prDomain, d.Status, commit, d.CreatedAt}
		}
		output.Table([]string{"PR", "BRANCH", "DOMAIN", "STATUS", "COMMIT", "CREATED"}, rows)
		fmt.Printf("\nTotal: %d active preview(s)\n", len(prs))
		return nil
	},
}

var projectsShellCmd = &cobra.Command{
	Use:   "shell [machine]",
	Short: "Connect to an interactive SPDY shell in the running pod",
	Long: `Upgrades your connection to an interactive SPDY-proxy WebSocket tunnel,
granting you direct /bin/sh access into the machine's running container securely.`,
	Example: `  usectl machines shell a8f15889`,
	Args:    cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}

		fmt.Printf("Connecting to project %s...\n", args[0])
		err = client.StreamTerminal(args[0], "")
		if err != nil {
			return err
		}
		return nil
	},
}

var projectsDiagnosticsCmd = &cobra.Command{
	Use:     "diagnostics [machine]",
	Short:   "View K8s crash reports, reasons, and previous logs for a failing pod",
	Long:    `Returns precise Kubernetes pod lifecycle events to debug crash loops or CreateContainerConfigErrors.`,
	Example: `  usectl machines diagnostics a8f15889`,
	Args:    cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if args, err = resolveFirstArg(client, args); err != nil {
			return err
		}

		diag, err := client.GetDiagnostics(args[0])
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(diag)
		}

		fmt.Printf("Diagnostics for Pod: %s\n", diag.PodName)
		fmt.Printf("Phase: %s\n", diag.Phase)

		if diag.ContainerStatus != nil {
			fmt.Printf("\nContainer Status: %s (%s)\n", diag.ContainerStatus.State, diag.ContainerStatus.WaitReason)
			if diag.ContainerStatus.Message != "" {
				fmt.Printf("Message: %s\n", diag.ContainerStatus.Message)
			}
			fmt.Printf("Restart Count: %d\n", diag.ContainerStatus.RestartCount)
		}

		if diag.PreviousLogs != "" {
			fmt.Println("\n--- Previous Crash Logs ---")
			fmt.Println(diag.PreviousLogs)
		}

		if len(diag.Events) > 0 {
			fmt.Println("\n--- Recent Kubernetes Events ---")
			rows := make([][]string, len(diag.Events))
			for i, e := range diag.Events {
				rows[i] = []string{e.Time, e.Type, e.Reason, e.Message}
			}
			output.Table([]string{"Time", "Type", "Reason", "Message"}, rows)
		} else {
			fmt.Println("\nNo recent adverse Kubernetes events found.")
		}

		return nil
	},
}

func init() {
	// Create flags
	projectsCreateCmd.Flags().StringVar(&createName, "name", "", "Machine name (or pass it positionally)")
	projectsCreateCmd.Flags().Float64Var(&createVCPU, "vcpu", 1, "vCPU allocated to the machine")
	projectsCreateCmd.Flags().StringVar(&createRAM, "ram", "1", "RAM — bare number is GB; suffixes accepted (512mb, 4gb)")
	projectsCreateCmd.Flags().StringVar(&createStorage, "storage", "1", "Storage — bare number is GB; suffixes accepted (512mb, 20gb)")
	projectsCreateCmd.Flags().StringVar(&createBilling, "billing", "month", "Billing interval: month or year")
	projectsCreateCmd.Flags().BoolVar(&createDB, "db", false, "Provision a PostgreSQL addon")
	projectsCreateCmd.Flags().BoolVar(&createS3, "s3", false, "Provision an S3 object-storage addon")
	projectsCreateCmd.Flags().StringSliceVar(&createAddons, "addon", nil, "Add addon by type (database, s3, redis, nats). Can be repeated")
	projectsCreateCmd.Flags().StringVar(&createGHToken, "github-token", "", "GitHub token used by pods in this machine for private repos")
	projectsCreateCmd.Flags().Int64Var(&createInstallID, "installation-id", 0, "GitHub App installation ID (from 'usectl github installations')")
	projectsCreateCmd.Flags().StringSliceVar(&createEnvs, "env", nil, `Machine-wide environment variable (KEY=value). Can be repeated`)
	projectsCreateCmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "Do not prompt; fail instead if a required value is missing")
	projectsDeleteCmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "Skip the delete confirmation")

	// Update flags
	projectsUpdateCmd.Flags().StringVar(&updateName, "name", "", "New project name")
	projectsUpdateCmd.Flags().StringVar(&updateDomain, "domain", "", "New subdomain")
	projectsUpdateCmd.Flags().StringVar(&updateBranch, "branch", "", "New branch")
	projectsUpdateCmd.Flags().IntVar(&updatePort, "port", 0, "New container port")
	projectsUpdateCmd.Flags().StringVar(&updateGHToken, "github-token", "", "New GitHub token")
	projectsUpdateCmd.Flags().Int64Var(&updateInstallID, "installation-id", 0, "GitHub App installation ID")
	projectsUpdateCmd.Flags().BoolVar(&updatePreviewEnvs, "preview-envs", false, "Enable or disable PR preview environments")

	// Logs flags
	projectsLogsCmd.Flags().IntVar(&logsLines, "tail", 100, "Number of log lines")
	projectsLogsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output in real-time")

	// Wire subcommands
	projectsCmd.AddCommand(projectsListCmd)
	projectsCmd.AddCommand(projectsGetCmd)
	projectsCmd.AddCommand(projectsCreateCmd)
	projectsCmd.AddCommand(projectsUpdateCmd)
	projectsCmd.AddCommand(projectsDeleteCmd)
	projectsCmd.AddCommand(projectsDeployCmd)
	projectsDeploymentsCmd.Flags().StringVar(&depsStatus, "status", "", "Filter by status: live, failed, building, cancelled")
	projectsDeploymentsCmd.Flags().IntVar(&depsPage, "page", 0, "Page number")
	projectsDeploymentsCmd.Flags().IntVar(&depsPerPage, "per-page", 0, "Results per page (max 100)")
	projectsCmd.AddCommand(projectsDeploymentsCmd)
	projectsCmd.AddCommand(projectsRollbackCmd)
	projectsCmd.AddCommand(projectsStartCmd)
	projectsCmd.AddCommand(projectsStopCmd)
	projectsCmd.AddCommand(projectsStatusCmd)
	projectsCmd.AddCommand(projectsLogsCmd)
	projectsCmd.AddCommand(projectsShellCmd)
	projectsCmd.AddCommand(projectsDiagnosticsCmd)
	projectsCmd.AddCommand(projectsBuildLogsCmd)
	projectsCmd.AddCommand(projectsStatsCmd)
	projectsCmd.AddCommand(projectsPRsCmd)
	// Additional sub-groups
	// s3Cmd is parented in s3.go; registering it here too duplicated it in help.
	projectsCmd.AddCommand(cronCmd)
	projectsCmd.AddCommand(envsCmd)
	projectsCmd.AddCommand(domainsCmd)

	rootCmd.AddCommand(projectsCmd)
}

// billingSummary renders a machine's billing state compactly for the list
// table: the price when there is one, the trial/other state otherwise.
func billingSummary(p api.Project) string {
	switch p.BillingStatus {
	case "trialing":
		return "trial"
	case "":
		return "-"
	}
	if p.MonthlyPriceCents > 0 {
		return fmt.Sprintf("$%.0f/%s", float64(p.MonthlyPriceCents)/100, shortInterval(p.BillingInterval))
	}
	return p.BillingStatus
}

func shortInterval(s string) string {
	if s == "year" {
		return "yr"
	}
	return "mo"
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// deployStatusColored maps a deployment status to a colour. Only "live" is a
// healthy resting state; "failed"/"cancelled" are terminal; the rest are in
// flight.
func deployStatusColored(s string) string {
	switch s {
	case "live":
		return output.Green(s)
	case "failed":
		return output.Red(s)
	case "cancelled":
		return output.Yellow(s)
	case "-", "":
		return output.Dim("-")
	default:
		return output.Yellow(s)
	}
}
