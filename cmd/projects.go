package cmd

import (
	"fmt"
	"strconv"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

var projectsCmd = &cobra.Command{
	Use:     "projects",
	Aliases: []string{"project", "p"},
	Short:   "Manage projects",
}

var projectsListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all projects",
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
			rows[i] = []string{p.ID[:8], p.Name, p.Domain, p.ProjectType, status, features, p.Branch}
		}
		output.Table([]string{"ID", "NAME", "DOMAIN", "TYPE", "STATUS", "FEATURES", "BRANCH"}, rows)
		return nil
	},
}

var projectsGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get project details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		project, err := client.GetProject(args[0])
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(project)
		}

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

		output.Table([]string{"FIELD", "VALUE"}, [][]string{
			{"ID", project.ID},
			{"Name", project.Name},
			{"Repo", project.RepoURL},
			{"Branch", project.Branch},
			{"Domain", project.Domain + ".usectl.com"},
			{"Type", project.ProjectType},
			{"Port", strconv.Itoa(project.Port)},
			{"Database", dbStatus},
			{"Object Storage", s3Status},
			{"Created", project.CreatedAt},
		})
		return nil
	},
}

// Flags for create command
var (
	createName    string
	createRepo    string
	createBranch  string
	createDomain  string
	createType    string
	createPort    int
	createDB      bool
	createS3      bool
	createGHToken string
)

var projectsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new project",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}

		req := api.CreateProjectRequest{
			Name:        createName,
			RepoURL:     createRepo,
			Branch:      createBranch,
			Domain:      createDomain,
			ProjectType: createType,
			Port:        createPort,
			NeedsDB:     createDB,
			NeedsS3:     createS3,
			GithubToken: createGHToken,
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
	updateName    string
	updateDomain  string
	updateBranch  string
	updatePort    int
	updateGHToken string
)

var projectsUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a project",
	Args:  cobra.ExactArgs(1),
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
	Short: "Delete a project",
	Args:  cobra.ExactArgs(1),
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
	Short: "Trigger a new deployment",
	Args:  cobra.ExactArgs(1),
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

var projectsLogsCmd = &cobra.Command{
	Use:   "logs <id>",
	Short: "View runtime logs",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
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
	Short: "View build/deploy logs for a specific deployment",
	Args:  cobra.ExactArgs(2),
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

var projectsStartCmd = &cobra.Command{
	Use:   "start <id>",
	Short: "Start a stopped project",
	Args:  cobra.ExactArgs(1),
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
	Use:   "stop <id>",
	Short: "Stop a running project",
	Args:  cobra.ExactArgs(1),
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
	Use:   "status <id>",
	Short: "Check project container status",
	Args:  cobra.ExactArgs(1),
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
	Short: "View resource usage stats",
	Args:  cobra.ExactArgs(1),
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
	projectsCreateCmd.Flags().StringVar(&createGHToken, "github-token", "", "GitHub token for private repos")
	projectsCreateCmd.MarkFlagRequired("name")
	projectsCreateCmd.MarkFlagRequired("repo")
	projectsCreateCmd.MarkFlagRequired("domain")

	// Update flags
	projectsUpdateCmd.Flags().StringVar(&updateName, "name", "", "New project name")
	projectsUpdateCmd.Flags().StringVar(&updateDomain, "domain", "", "New subdomain")
	projectsUpdateCmd.Flags().StringVar(&updateBranch, "branch", "", "New branch")
	projectsUpdateCmd.Flags().IntVar(&updatePort, "port", 0, "New container port")
	projectsUpdateCmd.Flags().StringVar(&updateGHToken, "github-token", "", "New GitHub token")

	// Logs flags
	projectsLogsCmd.Flags().IntVar(&logsLines, "tail", 100, "Number of log lines")

	// Wire subcommands
	projectsCmd.AddCommand(projectsListCmd)
	projectsCmd.AddCommand(projectsGetCmd)
	projectsCmd.AddCommand(projectsCreateCmd)
	projectsCmd.AddCommand(projectsUpdateCmd)
	projectsCmd.AddCommand(projectsDeleteCmd)
	projectsCmd.AddCommand(projectsDeployCmd)
	projectsCmd.AddCommand(projectsLogsCmd)
	projectsCmd.AddCommand(projectsBuildLogsCmd)
	projectsCmd.AddCommand(projectsStartCmd)
	projectsCmd.AddCommand(projectsStopCmd)
	projectsCmd.AddCommand(projectsStatusCmd)
	projectsCmd.AddCommand(projectsStatsCmd)

	rootCmd.AddCommand(projectsCmd)
}
