package cmd

import (
	"fmt"
	"strconv"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "Admin commands (user management)",
}

var adminUsersCmd = &cobra.Command{
	Use:   "users",
	Short: "Manage users",
}

var adminUsersListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all users",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		users, err := client.ListUsers()
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(users)
		}

		rows := make([][]string, len(users))
		for i, u := range users {
			rows[i] = []string{u.ID[:8], u.Username, u.Email, u.Role, strconv.FormatBool(u.Enabled)}
		}
		output.Table([]string{"ID", "USERNAME", "EMAIL", "ROLE", "ENABLED"}, rows)
		return nil
	},
}

var adminUsersEnableCmd = &cobra.Command{
	Use:   "enable <id>",
	Short: "Enable a user",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if err := client.SetUserEnabled(args[0], true); err != nil {
			return err
		}
		fmt.Println("✓ User enabled")
		return nil
	},
}

var adminUsersDisableCmd = &cobra.Command{
	Use:   "disable <id>",
	Short: "Disable a user",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if err := client.SetUserEnabled(args[0], false); err != nil {
			return err
		}
		fmt.Println("✓ User disabled")
		return nil
	},
}

var adminUsersSetRoleCmd = &cobra.Command{
	Use:   "set-role <id> <role>",
	Short: "Set user role (user or admin)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if err := client.SetUserRole(args[0], args[1]); err != nil {
			return err
		}
		fmt.Printf("✓ User role set to %s\n", args[1])
		return nil
	},
}

var adminUsersDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a user",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if err := client.DeleteUser(args[0]); err != nil {
			return err
		}
		fmt.Println("✓ User deleted")
		return nil
	},
}

func init() {
	adminUsersCmd.AddCommand(adminUsersListCmd)
	adminUsersCmd.AddCommand(adminUsersEnableCmd)
	adminUsersCmd.AddCommand(adminUsersDisableCmd)
	adminUsersCmd.AddCommand(adminUsersSetRoleCmd)
	adminUsersCmd.AddCommand(adminUsersDeleteCmd)

	adminCmd.AddCommand(adminUsersCmd)
	rootCmd.AddCommand(adminCmd)
}
