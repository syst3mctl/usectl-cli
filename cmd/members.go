package cmd

import (
	"fmt"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

var membersCmd = &cobra.Command{
	Use:     "members",
	Aliases: []string{"member", "team"},
	Short:   "Manage machine members and per-machine invitations",
	Long: `Project members are independent of organization roles — a user can be
an org admin with viewer-only access to a specific project, or vice versa.

Roles:
  owner      Full control (incl. billing, delete, member management)
  developer  Can deploy, manage envs, addons, apps. Optionally restricted
             to a whitelist of apps/addons via 'usectl members scope'.
  viewer     Read-only; sees masked secrets and metrics.`,
}

var membersListCmd = &cobra.Command{
	Use:   "list <project-id>",
	Short: "List members of a machine",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		members, err := client.ListProjectMembers(args[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(members)
		}
		if len(members) == 0 {
			fmt.Println("No members.")
			return nil
		}
		rows := make([][]string, len(members))
		for i, m := range members {
			rows[i] = []string{m.UserID, m.UserName, m.UserEmail, m.Role, m.CreatedAt}
		}
		output.Table([]string{"USER ID", "NAME", "EMAIL", "ROLE", "JOINED"}, rows)
		return nil
	},
}

var membersRoleCmd = &cobra.Command{
	Use:   "role <project-id> <user-id> <role>",
	Short: "Change a member's role (owner|developer|viewer). Owner-only.",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if err := client.UpdateProjectMemberRole(args[0], args[1], args[2]); err != nil {
			return err
		}
		fmt.Printf("✓ Role updated to %s\n", args[2])
		return nil
	},
}

var membersRemoveCmd = &cobra.Command{
	Use:     "remove <project-id> <user-id>",
	Aliases: []string{"rm"},
	Short:   "Remove a member from a machine. Owner-only.",
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if err := client.RemoveProjectMember(args[0], args[1]); err != nil {
			return err
		}
		fmt.Println("✓ Member removed")
		return nil
	},
}

var membersMyRoleCmd = &cobra.Command{
	Use:   "my-role <project-id>",
	Short: "Show my effective role on a project (and resource scope, if any)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		info, err := client.GetMyProjectRole(args[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(info)
		}
		fmt.Printf("Role: %s\n", info.Role)
		if info.Restricted {
			fmt.Println("Scope: restricted to:")
			for _, r := range info.Resources {
				fmt.Printf("  - %s %s\n", r.Kind, r.ID)
			}
		}
		return nil
	},
}

var membersScopeGetCmd = &cobra.Command{
	Use:   "scope-get <project-id> <user-id>",
	Short: "Show the developer's resource whitelist (apps/addons)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		rs, err := client.GetMemberResources(args[0], args[1])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(rs)
		}
		if len(rs) == 0 {
			fmt.Println("(unrestricted — full developer access to project)")
			return nil
		}
		rows := make([][]string, len(rs))
		for i, r := range rs {
			rows[i] = []string{r.Kind, r.ID}
		}
		output.Table([]string{"KIND", "ID"}, rows)
		return nil
	},
}

var (
	scopeApps   []string
	scopeAddons []string
	scopeClear  bool
)

var membersScopeSetCmd = &cobra.Command{
	Use:   "scope-set <project-id> <user-id>",
	Short: "Replace a developer's resource whitelist (use --clear to remove all restrictions)",
	Long: `Sets the developer's allowed apps/addons. Pass --app/--addon flags
(repeatable) to grant access to specific resources. The whitelist is
replaced atomically — pass --clear to remove all restrictions and grant
unrestricted developer access.`,
	Example: `  usectl members scope-set proj user --app app-id-1 --app app-id-2 --addon addon-id-1
  usectl members scope-set proj user --clear`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		var rs []api.MemberResource
		if !scopeClear {
			for _, id := range scopeApps {
				rs = append(rs, api.MemberResource{Kind: "app", ID: id})
			}
			for _, id := range scopeAddons {
				rs = append(rs, api.MemberResource{Kind: "addon", ID: id})
			}
		}
		if err := client.PutMemberResources(args[0], args[1], rs); err != nil {
			return err
		}
		if scopeClear || len(rs) == 0 {
			fmt.Println("✓ Scope cleared (unrestricted)")
		} else {
			fmt.Printf("✓ Scope set to %d resource(s)\n", len(rs))
		}
		return nil
	},
}

// --- Project invitations ---

var membersInviteCmd = &cobra.Command{
	Use:   "invitations",
	Aliases: []string{"invites"},
	Short: "Manage machine invitations (per-machine, separate from org invites)",
}

var (
	projInviteEmail string
	projInviteRole  string
)

var membersInviteCreateCmd = &cobra.Command{
	Use:   "create <project-id>",
	Short: "Invite a user to a machine by email. Owner-only.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		inv, err := client.CreateProjectInvitation(args[0], projInviteEmail, projInviteRole)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(inv)
		}
		fmt.Printf("✓ Invitation created\n")
		fmt.Printf("  Email:   %s\n", inv.Email)
		fmt.Printf("  Role:    %s\n", inv.Role)
		fmt.Printf("  Expires: %s\n", inv.ExpiresAt)
		fmt.Printf("  Token:   %s\n", inv.Token)
		fmt.Printf("\nShare link: https://usectl.com/project-invitations/%s\n", inv.Token)
		return nil
	},
}

var membersInviteListCmd = &cobra.Command{
	Use:   "list <project-id>",
	Short: "List pending invitations on a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		invs, err := client.ListProjectInvitations(args[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(invs)
		}
		if len(invs) == 0 {
			fmt.Println("No pending invitations.")
			return nil
		}
		rows := make([][]string, len(invs))
		for i, v := range invs {
			rows[i] = []string{v.ID, v.Email, v.Role, v.ExpiresAt}
		}
		output.Table([]string{"ID", "EMAIL", "ROLE", "EXPIRES"}, rows)
		return nil
	},
}

var membersInviteRevokeCmd = &cobra.Command{
	Use:   "revoke <project-id> <invitation-id>",
	Short: "Revoke a pending invitation. Owner-only.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if err := client.RevokeProjectInvitation(args[0], args[1]); err != nil {
			return err
		}
		fmt.Println("✓ Invitation revoked")
		return nil
	},
}

var membersInviteAcceptCmd = &cobra.Command{
	Use:   "accept <token>",
	Short: "Accept a project invitation token (you must be logged in)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		// Strip a full URL if pasted in.
		token := args[0]
		if i := strings.LastIndex(token, "/"); i >= 0 {
			token = token[i+1:]
		}
		info, err := client.AcceptProjectInvitation(token)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(info)
		}
		fmt.Printf("✓ Joined %q as %s\n", info.ProjectName, info.Role)
		return nil
	},
}

func init() {
	membersInviteCreateCmd.Flags().StringVar(&projInviteEmail, "email", "", "Email address (required)")
	membersInviteCreateCmd.Flags().StringVar(&projInviteRole, "role", "developer", "Role: owner | developer | viewer")
	membersInviteCreateCmd.MarkFlagRequired("email")

	membersScopeSetCmd.Flags().StringSliceVar(&scopeApps, "app", nil, "App UUID (repeatable)")
	membersScopeSetCmd.Flags().StringSliceVar(&scopeAddons, "addon", nil, "Addon UUID (repeatable)")
	membersScopeSetCmd.Flags().BoolVar(&scopeClear, "clear", false, "Clear all restrictions")

	membersInviteCmd.AddCommand(membersInviteCreateCmd, membersInviteListCmd,
		membersInviteRevokeCmd, membersInviteAcceptCmd)

	membersCmd.AddCommand(membersListCmd, membersRoleCmd, membersRemoveCmd,
		membersMyRoleCmd, membersScopeGetCmd, membersScopeSetCmd, membersInviteCmd)
	rootCmd.AddCommand(membersCmd)
}
