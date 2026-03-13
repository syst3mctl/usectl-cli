package cmd

import (
	"fmt"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

// ==================== Parent command ====================

var orgsCmd = &cobra.Command{
	Use:     "orgs",
	Aliases: []string{"org", "organizations"},
	Short:   "Manage organizations, members, and invitations",
	Long: `Manage organizations for team-based project collaboration.

Organizations let you group projects and team members together with
role-based access control (owner, admin, member).

Examples:
  usectl orgs list                         # List your organizations
  usectl orgs create --name "Acme Inc"     # Create a new organization
  usectl orgs members list <org-id>        # List members
  usectl orgs invite <org-id> --email a@b  # Invite a user
  usectl orgs projects <org-id>            # List organization projects`,
}

// ==================== orgs list ====================

var orgsListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List organizations you belong to",
	Long:    "Show all organizations the current user is a member of, including their role in each.",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		orgs, err := client.ListOrganizations()
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(orgs)
		}

		if len(orgs) == 0 {
			fmt.Println("No organizations found. Create one with: usectl orgs create --name \"My Org\"")
			return nil
		}

		rows := make([][]string, len(orgs))
		for i, o := range orgs {
			rows[i] = []string{o.ID[:8], o.Name, o.Slug, o.CreatedAt[:10]}
		}
		output.Table([]string{"ID", "NAME", "SLUG", "CREATED"}, rows)
		return nil
	},
}

// ==================== orgs get ====================

var orgsGetCmd = &cobra.Command{
	Use:   "get <org-id>",
	Short: "Get organization details",
	Long:  "Display detailed information about an organization including name, slug, and creation date.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		org, err := client.GetOrganization(args[0])
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(org)
		}

		output.Table([]string{"FIELD", "VALUE"}, [][]string{
			{"ID", org.ID},
			{"Name", org.Name},
			{"Slug", org.Slug},
			{"Description", org.Description},
			{"Created", org.CreatedAt},
		})
		return nil
	},
}

// ==================== orgs create ====================

var createOrgName string
var createOrgSlug string
var createOrgDesc string

var orgsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new organization",
	Long: `Create a new organization. You will be the owner and can invite others.

The slug is auto-generated from the name if not provided. It must be
unique and contain only lowercase letters, numbers, and hyphens.

Examples:
  usectl orgs create --name "Acme Inc"
  usectl orgs create --name "Acme Inc" --slug acme --desc "Our team org"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		org, err := client.CreateOrganization(api.CreateOrganizationRequest{
			Name:        createOrgName,
			Slug:        createOrgSlug,
			Description: createOrgDesc,
		})
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(org)
		}

		fmt.Printf("✓ Organization created: %s (ID: %s, slug: %s)\n", org.Name, org.ID[:8], org.Slug)
		return nil
	},
}

// ==================== orgs update ====================

var updateOrgName string
var updateOrgDesc string

var orgsUpdateCmd = &cobra.Command{
	Use:   "update <org-id>",
	Short: "Update an organization's name or description",
	Long: `Update organization details. Only owners and admins can update.

Examples:
  usectl orgs update <org-id> --name "New Name"
  usectl orgs update <org-id> --desc "Updated description"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}

		req := api.UpdateOrganizationRequest{}
		if cmd.Flags().Changed("name") {
			req.Name = &updateOrgName
		}
		if cmd.Flags().Changed("desc") {
			req.Description = &updateOrgDesc
		}

		org, err := client.UpdateOrganization(args[0], req)
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(org)
		}

		fmt.Printf("✓ Organization updated: %s\n", org.Name)
		return nil
	},
}

// ==================== orgs delete ====================

var orgsDeleteCmd = &cobra.Command{
	Use:   "delete <org-id>",
	Short: "Delete an organization",
	Long: `Permanently delete an organization and remove all member associations.
Projects belonging to the organization will be unlinked but not deleted.

Only the organization owner can perform this action.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if err := client.DeleteOrganization(args[0]); err != nil {
			return err
		}
		fmt.Println("✓ Organization deleted")
		return nil
	},
}

// ==================== orgs projects ====================

var orgsProjectsCmd = &cobra.Command{
	Use:     "projects <org-id>",
	Aliases: []string{"proj"},
	Short:   "List projects belonging to an organization",
	Long:    "Show all projects that belong to the specified organization. All organization members can view these projects.",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		projects, err := client.ListOrgProjects(args[0])
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(projects)
		}

		if len(projects) == 0 {
			fmt.Println("No projects in this organization.")
			return nil
		}

		rows := make([][]string, len(projects))
		for i, p := range projects {
			rows[i] = []string{p.ID[:8], p.Name, p.Domain, p.Branch, p.ProjectType}
		}
		output.Table([]string{"ID", "NAME", "DOMAIN", "BRANCH", "TYPE"}, rows)
		return nil
	},
}

// ==================== Members subgroup ====================

var orgsMembersCmd = &cobra.Command{
	Use:     "members",
	Aliases: []string{"member"},
	Short:   "Manage organization members",
	Long:    "List, update roles, and remove members from an organization.",
}

// ---- members list ----

var orgsMembersListCmd = &cobra.Command{
	Use:     "list <org-id>",
	Aliases: []string{"ls"},
	Short:   "List members of an organization",
	Long:    "Show all members of the specified organization with their roles and join dates.",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		members, err := client.ListOrgMembers(args[0])
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(members)
		}

		if len(members) == 0 {
			fmt.Println("No members found.")
			return nil
		}

		rows := make([][]string, len(members))
		for i, m := range members {
			rows[i] = []string{m.UserID[:8], m.Username, m.Email, m.Role, m.JoinedAt[:10]}
		}
		output.Table([]string{"USER ID", "USERNAME", "EMAIL", "ROLE", "JOINED"}, rows)
		return nil
	},
}

// ---- members set-role ----

var setRoleValue string

var orgsMembersSetRoleCmd = &cobra.Command{
	Use:   "set-role <org-id> <user-id>",
	Short: "Change a member's role in an organization",
	Long: `Change the role of an organization member.

Available roles: owner, admin, member, viewer

Examples:
  usectl orgs members set-role <org-id> <user-id> --role admin
  usectl orgs members set-role <org-id> <user-id> --role viewer`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		err = client.UpdateMemberRole(args[0], args[1], api.UpdateMemberRoleRequest{
			Role: setRoleValue,
		})
		if err != nil {
			return err
		}
		fmt.Printf("✓ Role updated to %s\n", setRoleValue)
		return nil
	},
}

// ---- members remove ----

var orgsMembersRemoveCmd = &cobra.Command{
	Use:   "remove <org-id> <user-id>",
	Short: "Remove a member from an organization",
	Long: `Remove a user from the organization. The last owner cannot be removed.

Examples:
  usectl orgs members remove <org-id> <user-id>`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if err := client.RemoveMember(args[0], args[1]); err != nil {
			return err
		}
		fmt.Println("✓ Member removed")
		return nil
	},
}

// ==================== Invitations subgroup ====================

var orgsInviteCmd = &cobra.Command{
	Use:     "invite",
	Aliases: []string{"invitations", "inv"},
	Short:   "Manage organization invitations",
	Long:    "Create, list, and revoke invitations to an organization.",
}

// ---- invite list ----

var orgsInviteListCmd = &cobra.Command{
	Use:     "list <org-id>",
	Aliases: []string{"ls"},
	Short:   "List pending invitations",
	Long:    "Show all pending (not yet accepted) invitations for the specified organization.",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		invs, err := client.ListInvitations(args[0])
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
		for i, inv := range invs {
			rows[i] = []string{inv.ID[:8], inv.Email, inv.Role, inv.ExpiresAt[:10], inv.Token[:12] + "..."}
		}
		output.Table([]string{"ID", "EMAIL", "ROLE", "EXPIRES", "TOKEN"}, rows)
		return nil
	},
}

// ---- invite create ----

var inviteEmail string
var inviteRole string

var orgsInviteCreateCmd = &cobra.Command{
	Use:   "create <org-id>",
	Short: "Invite a user to an organization",
	Long: `Send an invitation to a user by email address. The invitation creates a
shareable link that the user can use to join the organization.

The invitation expires after 7 days. The default role is "member".
Available roles: admin, member, viewer (owner cannot be invited directly).

Examples:
  usectl orgs invite create <org-id> --email user@example.com
  usectl orgs invite create <org-id> --email user@example.com --role admin`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		inv, err := client.CreateInvitation(args[0], api.CreateInvitationRequest{
			Email: inviteEmail,
			Role:  inviteRole,
		})
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(inv)
		}

		fmt.Printf("✓ Invitation sent to %s (role: %s)\n", inv.Email, inv.Role)
		fmt.Printf("  Token: %s\n", inv.Token)
		fmt.Printf("  Expires: %s\n", inv.ExpiresAt[:10])
		return nil
	},
}

// ---- invite revoke ----

var orgsInviteRevokeCmd = &cobra.Command{
	Use:     "revoke <org-id> <invitation-id>",
	Aliases: []string{"delete", "rm"},
	Short:   "Revoke a pending invitation",
	Long: `Cancel a pending invitation so it can no longer be used to join the organization.

Examples:
  usectl orgs invite revoke <org-id> <invitation-id>`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if err := client.RevokeInvitation(args[0], args[1]); err != nil {
			return err
		}
		fmt.Println("✓ Invitation revoked")
		return nil
	},
}

// ---- invite info ----

var orgsInviteInfoCmd = &cobra.Command{
	Use:   "info <token>",
	Short: "View details of a pending invitation",
	Long: `Display information about a pending invitation before accepting it.
Shows the organization name, invited email, role, and expiry date.

Examples:
  usectl orgs invite info <token>`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		info, err := client.GetInvitationInfo(args[0])
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(info)
		}

		output.Table([]string{"FIELD", "VALUE"}, [][]string{
			{"Organization", info.OrgName},
			{"Org ID", info.OrganizationID},
			{"Email", info.Email},
			{"Role", info.Role},
			{"Expires", info.ExpiresAt[:10]},
		})
		fmt.Println("\nTo accept: usectl orgs invite accept <token>")
		return nil
	},
}

// ---- invite accept ----

var orgsInviteAcceptCmd = &cobra.Command{
	Use:   "accept <token>",
	Short: "Accept a pending invitation and join the organization",
	Long: `Accept a pending invitation using the token. You will be added as a member
of the organization with the role specified in the invitation.

Use 'usectl orgs invite info <token>' to review details before accepting.

Examples:
  usectl orgs invite accept <token>`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		resp, err := client.AcceptInvitation(args[0])
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(resp)
		}

		fmt.Printf("✓ Invitation accepted — you are now a member of %s\n", resp.OrgName)
		fmt.Println("  View your organizations: usectl orgs list")
		return nil
	},
}

// ==================== init ====================

func init() {
	// orgs create flags
	orgsCreateCmd.Flags().StringVar(&createOrgName, "name", "", "Organization name (required)")
	orgsCreateCmd.Flags().StringVar(&createOrgSlug, "slug", "", "URL-friendly slug (auto-generated from name if omitted)")
	orgsCreateCmd.Flags().StringVar(&createOrgDesc, "desc", "", "Organization description")
	orgsCreateCmd.MarkFlagRequired("name")

	// orgs update flags
	orgsUpdateCmd.Flags().StringVar(&updateOrgName, "name", "", "New organization name")
	orgsUpdateCmd.Flags().StringVar(&updateOrgDesc, "desc", "", "New description")

	// members set-role flags
	orgsMembersSetRoleCmd.Flags().StringVar(&setRoleValue, "role", "", "Role to assign: owner, admin, member, or viewer (required)")
	orgsMembersSetRoleCmd.MarkFlagRequired("role")

	// invite create flags
	orgsInviteCreateCmd.Flags().StringVar(&inviteEmail, "email", "", "Email address of the user to invite (required)")
	orgsInviteCreateCmd.Flags().StringVar(&inviteRole, "role", "member", "Role to assign: admin, member, or viewer")
	orgsInviteCreateCmd.MarkFlagRequired("email")

	// Wire members subcommands
	orgsMembersCmd.AddCommand(orgsMembersListCmd)
	orgsMembersCmd.AddCommand(orgsMembersSetRoleCmd)
	orgsMembersCmd.AddCommand(orgsMembersRemoveCmd)

	// Wire invite subcommands
	orgsInviteCmd.AddCommand(orgsInviteListCmd)
	orgsInviteCmd.AddCommand(orgsInviteCreateCmd)
	orgsInviteCmd.AddCommand(orgsInviteRevokeCmd)
	orgsInviteCmd.AddCommand(orgsInviteInfoCmd)
	orgsInviteCmd.AddCommand(orgsInviteAcceptCmd)

	// Wire top-level subcommands
	orgsCmd.AddCommand(orgsListCmd)
	orgsCmd.AddCommand(orgsGetCmd)
	orgsCmd.AddCommand(orgsCreateCmd)
	orgsCmd.AddCommand(orgsUpdateCmd)
	orgsCmd.AddCommand(orgsDeleteCmd)
	orgsCmd.AddCommand(orgsProjectsCmd)
	orgsCmd.AddCommand(orgsMembersCmd)
	orgsCmd.AddCommand(orgsInviteCmd)

	rootCmd.AddCommand(orgsCmd)
}
