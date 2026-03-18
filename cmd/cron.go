package cmd

import (
	"fmt"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

// Flags
var (
	cronName     string
	cronSchedule string
	cronCommand  string
	cronEnabled  string
)

var cronCmd = &cobra.Command{
	Use:     "cron",
	Aliases: []string{"crons", "cronjob"},
	Short:   "Manage scheduled cron jobs for a project",
	Long: `Manage Kubernetes CronJobs for a project. Cron jobs run on a schedule
using the same container image as the project's deployment, with access
to all addon secrets (database, Redis, etc.).

Requires the 'cron' addon to be enabled on the project.`,
}

var cronListCmd = &cobra.Command{
	Use:     "list <project-id>",
	Aliases: []string{"ls"},
	Short:   "List all cron jobs for a project",
	Example: `  usectl cron list a8f15889
  usectl cron ls a8f15889 --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		crons, err := client.ListProjectCrons(args[0])
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(crons)
		}

		if len(crons) == 0 {
			fmt.Println("No cron jobs found for this project.")
			fmt.Println("\nHint: Add a cron job with:")
			fmt.Println("  usectl cron add <project-id> --name cleanup --schedule '*/5 * * * *' --command './cleanup.sh'")
			return nil
		}

		rows := make([][]string, len(crons))
		for i, c := range crons {
			enabled := "✓"
			if !c.Enabled {
				enabled = "✗"
			}
			lastRun := "-"
			if c.LastRunAt != nil {
				lastRun = *c.LastRunAt
			}
			rows[i] = []string{c.ID[:8], c.Name, c.Schedule, c.Command, enabled, c.LastStatus, lastRun}
		}
		output.Table([]string{"ID", "NAME", "SCHEDULE", "COMMAND", "ENABLED", "LAST STATUS", "LAST RUN"}, rows)
		return nil
	},
}

var cronAddCmd = &cobra.Command{
	Use:   "add <project-id>",
	Short: "Add a new cron job to a project",
	Long: `Create a scheduled cron job. The job runs in the same container image
as the project's deployment with access to all addon env vars.

Schedule format: standard cron expression (minute hour day month weekday).

Common presets:
  */5 * * * *    — Every 5 minutes
  0 * * * *      — Every hour
  0 0 * * *      — Daily at midnight
  0 2 * * *      — Daily at 2 AM
  0 0 * * 0      — Weekly on Sunday`,
	Example: `  usectl cron add a8f15889 --name cleanup --schedule "*/5 * * * *" --command "./cleanup.sh"
  usectl cron add a8f15889 --name backup --schedule "0 2 * * *" --command "npm run backup"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}

		cron, err := client.CreateProjectCron(args[0], api.CreateCronRequest{
			Name:     cronName,
			Schedule: cronSchedule,
			Command:  cronCommand,
		})
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(cron)
		}

		fmt.Printf("✓ Cron job created: %s (%s)\n", cron.Name, cron.ID[:8])
		fmt.Printf("  Schedule: %s\n", cron.Schedule)
		fmt.Printf("  Command:  %s\n", cron.Command)
		return nil
	},
}

var cronUpdateCmd = &cobra.Command{
	Use:   "update <project-id> <cron-id>",
	Short: "Update a cron job's schedule, command, or enabled state",
	Example: `  usectl cron update a8f15889 d4e5f6a7 --schedule "0 3 * * *"
  usectl cron update a8f15889 d4e5f6a7 --enabled false
  usectl cron update a8f15889 d4e5f6a7 --command "npm run new-task"`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}

		req := api.UpdateCronRequest{}
		if cmd.Flags().Changed("schedule") {
			req.Schedule = &cronSchedule
		}
		if cmd.Flags().Changed("command") {
			req.Command = &cronCommand
		}
		if cmd.Flags().Changed("enabled") {
			val := cronEnabled == "true"
			req.Enabled = &val
		}

		cron, err := client.UpdateProjectCron(args[0], args[1], req)
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(cron)
		}

		fmt.Printf("✓ Cron job updated: %s\n", cron.Name)
		return nil
	},
}

var cronDeleteCmd = &cobra.Command{
	Use:     "delete <project-id> <cron-id>",
	Aliases: []string{"rm", "remove"},
	Short:   "Delete a cron job",
	Example: `  usectl cron delete a8f15889 d4e5f6a7`,
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}

		if err := client.DeleteProjectCron(args[0], args[1]); err != nil {
			return err
		}
		fmt.Println("✓ Cron job deleted")
		return nil
	},
}

func init() {
	// Cron add flags
	cronAddCmd.Flags().StringVar(&cronName, "name", "", "Cron job name (required)")
	cronAddCmd.Flags().StringVar(&cronSchedule, "schedule", "", "Cron schedule expression (required)")
	cronAddCmd.Flags().StringVar(&cronCommand, "command", "", "Command to run (required)")
	cronAddCmd.MarkFlagRequired("name")
	cronAddCmd.MarkFlagRequired("schedule")
	cronAddCmd.MarkFlagRequired("command")

	// Cron update flags
	cronUpdateCmd.Flags().StringVar(&cronSchedule, "schedule", "", "New schedule")
	cronUpdateCmd.Flags().StringVar(&cronCommand, "command", "", "New command")
	cronUpdateCmd.Flags().StringVar(&cronEnabled, "enabled", "", "Enable or disable (true/false)")

	// Wire subcommands
	cronCmd.AddCommand(cronListCmd)
	cronCmd.AddCommand(cronAddCmd)
	cronCmd.AddCommand(cronUpdateCmd)
	cronCmd.AddCommand(cronDeleteCmd)

	rootCmd.AddCommand(cronCmd)
}
