package cmd

import (
	"fmt"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

var notificationsCmd = &cobra.Command{
	Use:     "notifications",
	Aliases: []string{"notify", "notifs"},
	Short:   "View and manage in-app notifications",
}

var notificationsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent notifications",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		notifs, err := client.ListNotifications()
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(notifs)
		}
		if len(notifs) == 0 {
			fmt.Println("No notifications.")
			return nil
		}
		rows := make([][]string, len(notifs))
		for i, n := range notifs {
			read := "yes"
			if !n.Read {
				read = "no"
			}
			rows[i] = []string{n.ID, n.Type, n.Title, read, n.CreatedAt}
		}
		output.Table([]string{"ID", "TYPE", "TITLE", "READ", "CREATED"}, rows)
		return nil
	},
}

var notificationsReadCmd = &cobra.Command{
	Use:   "read <id>",
	Short: "Mark a notification as read",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if err := client.MarkNotificationRead(args[0]); err != nil {
			return err
		}
		fmt.Println("✓ Marked as read")
		return nil
	},
}

var notificationsReadAllCmd = &cobra.Command{
	Use:   "read-all",
	Short: "Mark all notifications as read",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if err := client.MarkAllNotificationsRead(); err != nil {
			return err
		}
		fmt.Println("✓ All marked as read")
		return nil
	},
}

var notificationsCountCmd = &cobra.Command{
	Use:   "unread",
	Short: "Show unread notification count",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		n, err := client.UnreadNotificationCount()
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(map[string]int{"count": n})
		}
		fmt.Printf("%d unread\n", n)
		return nil
	},
}

func init() {
	notificationsCmd.AddCommand(notificationsListCmd, notificationsReadCmd,
		notificationsReadAllCmd, notificationsCountCmd)
	rootCmd.AddCommand(notificationsCmd)
}
