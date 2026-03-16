package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/config"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

var projectsCmd = &cobra.Command{
	Use:     "projects",
	Aliases: []string{"project", "p"},
	Short:   "Manage projects — create, deploy, update, delete, and monitor applications",
	Long: `Manage the full lifecycle of applications on the usectl platform.

A project represents a deployable application linked to a GitHub repository.
Each project gets its own Kubernetes namespace, optional PostgreSQL database,
and optional S3 storage bucket (MinIO).

Subcommands:
  list         List all projects with status and features
  get          Show detailed project info including deployments
  create       Create a new project from a GitHub repository
  update       Modify project settings (domain, branch, port)
  delete       Delete a project and all its resources (namespace, DB, S3)
  deploy       Trigger a new build and deployment
  deployments  List all deployments for a project
  rollback     Roll back to a previous deployment
  start/stop   Scale the project's containers up or down
  status       Check if the project's containers are running
  logs         View live runtime logs from the application
  build-logs   View build and deploy logs for a specific deployment
  stats        View CPU, memory, and network usage metrics
  s3           Manage S3 object storage for the project`,
}

var projectsListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all projects with their status, domain, and features",
	Long: `Returns a table of all projects the authenticated user has access to.
Admin users see all projects. Columns include ID, name, domain, type,
latest deployment status, enabled features (db/s3), and branch.

Use --json for structured output suitable for scripting or AI agents.`,
	Example: `  usectl projects list
  usectl projects list --json
  usectl p ls`,
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

		rows := make([][]string, len(projects))
		for i, pw := range projects {
			p := pw.Project
			features := ""
			if p.NeedsDB {
				features += "db"
			}
			if p.NeedsS3 {
				if features != "" {
					features += ","
				}
				features += "s3"
			}
			if features == "" {
				features = "-"
			}
			status := "-"
			if pw.LatestDeployment != nil {
				status = pw.LatestDeployment.Status
			}
			rows[i] = []string{p.ID, p.Name, p.Domain, p.ProjectType, status, features, p.Branch}
		}
		output.Table([]string{"ID", "NAME", "DOMAIN", "TYPE", "STATUS", "FEATURES", "BRANCH"}, rows)
		return nil
	},
}

var projectsGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get detailed project information including database and S3 status",
	Long: `Returns detailed information about a single project, including repo URL,
branch, domain, port, database provisioning status, S3 bucket status,
creation date, and recent deployment history.

The <id> can be the full UUID or a prefix (e.g. first 8 chars).`,
	Example: `  usectl projects get a8f15889
  usectl projects get a8f15889-3636-402d-99a1-3492ba6b4383 --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
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

		displayDomain := project.Domain + ".usectl.com"
		if strings.Contains(project.Domain, ".") {
			displayDomain = project.Domain
		}
		output.Table([]string{"FIELD", "VALUE"}, [][]string{
			{"ID", project.ID},
			{"Name", project.Name},
			{"Repo", project.RepoURL},
			{"Branch", project.Branch},
			{"Domain", displayDomain},
			{"Type", project.ProjectType},
			{"Port", strconv.Itoa(project.Port)},
			{"Database", dbStatus},
			{"Object Storage", s3Status},
			{"Created", project.CreatedAt},
		})

		// Show recent deployments.
		if len(resp.Deployments) > 0 {
			fmt.Println()
			fmt.Println("Recent Deployments:")
			limit := len(resp.Deployments)
			if limit > 5 {
				limit = 5
			}
			rows := make([][]string, limit)
			for i := 0; i < limit; i++ {
				d := resp.Deployments[i]
				commit := d.CommitHash
				if len(commit) > 7 {
					commit = commit[:7]
				}
				rows[i] = []string{d.ID, d.Status, commit, d.CreatedAt}
			}
			output.Table([]string{"DEPLOYMENT ID", "STATUS", "COMMIT", "CREATED"}, rows)
			if len(resp.Deployments) > 5 {
				fmt.Printf("  ... and %d more. Use 'usectl projects deployments %s' to see all.\n", len(resp.Deployments)-5, args[0])
			}
		}
		return nil
	},
}

// Flags for create command
var (
	createName      string
	createRepo      string
	createBranch    string
	createDomain    string
	createType      string
	createPort      int
	createDB        bool
	createS3        bool
	createGHToken   string
	createInstallID int64
	createAddons    []string
)

var projectsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new project from a GitHub repository",
	Long: `Create a new project linked to a GitHub repository. The project will be
assigned its own Kubernetes namespace (kdeploy-<name>) and can optionally
provision a PostgreSQL database (--db) and S3 bucket (--s3).

The GitHub App installation ID is auto-detected if you have previously run
'usectl github login'. For private repos, this ensures the build system
can clone using installation tokens.

Supported project types:
  service  — Long-running server (Node.js, Go, Python, etc.)
  static   — Static site served via nginx

The system auto-detects Dockerfile, Next.js, Vite, or Node.js projects
and injects an appropriate Dockerfile if none exists.`,
	Example: `  # Minimal service
  usectl projects create --name my-api --repo https://github.com/user/api --domain my-api --port 3000

  # Full-featured with database and S3
  usectl projects create --name my-app --repo https://github.com/user/app \\
    --domain my-app --type service --branch main --port 8080 --db --s3

  # Static site
  usectl projects create --name docs --repo https://github.com/user/docs \\
    --domain docs --type static`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}

		// Auto-detect installation ID if not provided.
		if createInstallID == 0 {
			cfg, _ := config.Load()
			if cfg != nil && cfg.GitHubToken != "" {
				installations, err := client.ListGitHubInstallations(cfg.GitHubToken)
				if err == nil && len(installations) > 0 {
					createInstallID = installations[0].ID
					fmt.Printf("  Auto-detected GitHub App installation: %d (%s)\n",
						createInstallID, installations[0].Account.Login)
				}
			}
		}

		// Build addons list from --addon flags and legacy --db/--s3.
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
		var addonsList []string
		for a := range allAddons {
			addonsList = append(addonsList, a)
		}

		req := api.CreateProjectRequest{
			Name:        createName,
			RepoURL:     createRepo,
			Branch:      createBranch,
			Domain:      createDomain,
			ProjectType: createType,
			Port:        createPort,
			NeedsDB:     createDB || allAddons["database"],
			NeedsS3:     createS3 || allAddons["s3"],
			GithubToken: createGHToken,
			Addons:      addonsList,
		}
		if createInstallID > 0 {
			req.InstallationID = &createInstallID
		}

		project, err := client.CreateProject(req)
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(project)
		}

		fmt.Printf("✓ Project created: %s (ID: %s)\n", project.Name, project.ID)
		fmt.Printf("  Domain: %s.usectl.com\n", project.Domain)
		return nil
	},
}

// Flags for update command
var (
	updateName      string
	updateDomain    string
	updateBranch    string
	updatePort      int
	updateGHToken   string
	updateInstallID int64
)

var projectsUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update project settings (domain, branch, port, etc.)",
	Long: `Modify one or more settings of an existing project. Only the flags you
provide will be updated — omitted fields remain unchanged.

If --port or --domain is changed, the K8s resources (Deployment, Service,
IngressRoute) are automatically updated in the background.`,
	Example: `  usectl projects update a8f15889 --port 3000
  usectl projects update a8f15889 --domain new-domain --branch develop
  usectl projects update a8f15889 --installation-id 114078944`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
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
	Use:   "delete <id>",
	Short: "Delete a project and all associated resources (namespace, DB, S3)",
	Long: `Permanently delete a project and clean up all associated resources:
  - Kubernetes namespace and all resources inside (pods, services, ingress)
  - Provisioned PostgreSQL database and user (if --db was used)
  - S3 bucket, objects, user, and policy (if --s3 was used)

This action is irreversible.`,
	Example: `  usectl projects delete a8f15889`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if err := client.DeleteProject(args[0]); err != nil {
			return err
		}
		fmt.Println("✓ Project deleted")
		return nil
	},
}

var projectsDeployCmd = &cobra.Command{
	Use:   "deploy <id>",
	Short: "Trigger a new build and deployment from the latest commit",
	Long: `Trigger a build pipeline for the project. The backend auto-resolves the
latest commit on the project's branch via the GitHub API, clones the repo,
builds a container image via Kaniko, pushes it to the private registry,
and deploys it to Kubernetes.

The build runs asynchronously. Use 'usectl projects logs <id>' or
'usectl projects build-logs <project-id> <deployment-id>' to monitor progress.`,
	Example: `  usectl projects deploy a8f15889`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		resp, err := client.DeployProject(args[0])
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(resp)
		}

		fmt.Printf("✓ Deployment triggered (ID: %s, status: %s)\n", resp.Deployment.ID, resp.Deployment.Status)
		return nil
	},
}

var logsLines int
var logsFollow bool

var projectsLogsCmd = &cobra.Command{
	Use:   "logs <id>",
	Short: "View live runtime logs from the running application containers",
	Long: `Fetch the latest log output from the project's running pods.
Use --tail to control how many lines to retrieve (default: 100).
Use -f / --follow to stream logs in real-time (like docker logs -f).`,
	Example: `  usectl projects logs a8f15889
  usectl projects logs a8f15889 --tail 500
  usectl projects logs a8f15889 -f
  usectl projects logs a8f15889 --follow --tail 50`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}

		// Follow mode: stream to stdout.
		if logsFollow {
			return client.StreamRuntimeLogs(args[0], logsLines, os.Stdout)
		}

		// Normal mode: fetch and print.
		logs, err := client.GetRuntimeLogs(args[0], logsLines)
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
	Use:   "build-logs <project-id> <deployment-id>",
	Short: "View build and deploy logs for a specific deployment",
	Long: `Retrieve the full build log (clone + Kaniko build) and deploy log for a
specific deployment. Use 'usectl projects get <id>' to see deployment IDs.`,
	Example: `  usectl projects build-logs a8f15889 d4e5f6a7`,
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		logs, err := client.GetDeploymentLogs(args[0], args[1])
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(logs)
		}

		if logs.BuildLog != "" {
			fmt.Println("=== Build Log ===")
			fmt.Println(logs.BuildLog)
		}
		if logs.DeployLog != "" {
			fmt.Println("=== Deploy Log ===")
			fmt.Println(logs.DeployLog)
		}
		return nil
	},
}

var projectsDeploymentsCmd = &cobra.Command{
	Use:     "deployments <project-id>",
	Aliases: []string{"deps"},
	Short:   "List all deployments for a project",
	Long: `Returns a table of all deployments for the given project, ordered from
newest to oldest. Shows deployment ID, status, commit hash, and creation date.

Use the deployment ID with 'usectl projects build-logs' to view build logs,
or with 'usectl projects rollback' to roll back to a previous deployment.`,
	Example: `  usectl projects deployments a8f15889
  usectl projects deployments a8f15889 --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		resp, err := client.GetProjectFull(args[0])
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(resp.Deployments)
		}

		if len(resp.Deployments) == 0 {
			fmt.Println("No deployments found for this project.")
			return nil
		}

		rows := make([][]string, len(resp.Deployments))
		for i, d := range resp.Deployments {
			commit := d.CommitHash
			if len(commit) > 7 {
				commit = commit[:7]
			}
			rows[i] = []string{d.ID, d.Status, commit, d.CreatedAt}
		}
		output.Table([]string{"ID", "STATUS", "COMMIT", "CREATED"}, rows)
		fmt.Printf("\nTotal: %d deployments\n", len(resp.Deployments))
		fmt.Println("\nHint: Use 'usectl projects build-logs <project-id> <deployment-id>' to view logs.")
		fmt.Println("      Use 'usectl projects rollback <project-id> <deployment-id>' to roll back.")
		return nil
	},
}

var projectsRollbackCmd = &cobra.Command{
	Use:   "rollback <project-id> <deployment-id>",
	Short: "Roll back to a previous deployment (redeploy its container image)",
	Long: `Roll back a project to a previously successful deployment by redeploying
its container image without rebuilding. This is useful to quickly recover from
a bad deployment.

The deployment-id should reference an existing deployment. Use
'usectl projects deployments <project-id>' to see available deployments.`,
	Example: `  usectl projects rollback a8f15889 d4e5f6a7`,
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
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
		fmt.Println("  Use 'usectl projects logs <project-id>' to monitor.")
		return nil
	},
}

var projectsStartCmd = &cobra.Command{
	Use:     "start <id>",
	Short:   "Start a stopped project (scale replicas to 1)",
	Example: `  usectl projects start a8f15889`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
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
	Use:     "stop <id>",
	Short:   "Stop a running project (scale replicas to 0)",
	Example: `  usectl projects stop a8f15889`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
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
	Use:     "status <id>",
	Short:   "Check if the project's containers are running or stopped",
	Long:    `Returns the running status and current replica count of the project's deployment.`,
	Example: `  usectl projects status a8f15889`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
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
	Use:   "stats <id>",
	Short: "View CPU, memory, network, database size, and storage usage",
	Long: `Returns resource usage metrics for the project, including:
  - Per-pod CPU and memory usage
  - Network RX/TX
  - Pod restart count
  - Database size (if provisioned)
  - S3 storage used (if provisioned)`,
	Example: `  usectl projects stats a8f15889
  usectl projects stats a8f15889 --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
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

func init() {
	// Create flags
	projectsCreateCmd.Flags().StringVar(&createName, "name", "", "Project name (required)")
	projectsCreateCmd.Flags().StringVar(&createRepo, "repo", "", "GitHub repository URL (required)")
	projectsCreateCmd.Flags().StringVar(&createBranch, "branch", "main", "Git branch")
	projectsCreateCmd.Flags().StringVar(&createDomain, "domain", "", "Subdomain (required)")
	projectsCreateCmd.Flags().StringVar(&createType, "type", "service", "Project type: static or service")
	projectsCreateCmd.Flags().IntVar(&createPort, "port", 80, "Container port")
	projectsCreateCmd.Flags().BoolVar(&createDB, "db", false, "Provision a PostgreSQL database")
	projectsCreateCmd.Flags().BoolVar(&createS3, "s3", false, "Provision S3 object storage (MinIO)")
	projectsCreateCmd.Flags().StringSliceVar(&createAddons, "addon", nil, "Add addon by type (database, s3, redis, nats). Can be repeated")
	projectsCreateCmd.Flags().StringVar(&createGHToken, "github-token", "", "GitHub token for private repos")
	projectsCreateCmd.Flags().Int64Var(&createInstallID, "installation-id", 0, "GitHub App installation ID (from 'usectl github installations')")
	projectsCreateCmd.MarkFlagRequired("name")
	projectsCreateCmd.MarkFlagRequired("repo")
	projectsCreateCmd.MarkFlagRequired("domain")

	// Update flags
	projectsUpdateCmd.Flags().StringVar(&updateName, "name", "", "New project name")
	projectsUpdateCmd.Flags().StringVar(&updateDomain, "domain", "", "New subdomain")
	projectsUpdateCmd.Flags().StringVar(&updateBranch, "branch", "", "New branch")
	projectsUpdateCmd.Flags().IntVar(&updatePort, "port", 0, "New container port")
	projectsUpdateCmd.Flags().StringVar(&updateGHToken, "github-token", "", "New GitHub token")
	projectsUpdateCmd.Flags().Int64Var(&updateInstallID, "installation-id", 0, "GitHub App installation ID")

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
	projectsCmd.AddCommand(projectsLogsCmd)
	projectsCmd.AddCommand(projectsBuildLogsCmd)
	projectsCmd.AddCommand(projectsDeploymentsCmd)
	projectsCmd.AddCommand(projectsRollbackCmd)
	projectsCmd.AddCommand(projectsStartCmd)
	projectsCmd.AddCommand(projectsStopCmd)
	projectsCmd.AddCommand(projectsStatusCmd)
	projectsCmd.AddCommand(projectsStatsCmd)

	rootCmd.AddCommand(projectsCmd)
}
