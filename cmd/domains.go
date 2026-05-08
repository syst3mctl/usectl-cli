package cmd

import (
	"fmt"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

var domainsCmd = &cobra.Command{
	Use:     "domains",
	Aliases: []string{"domain", "d"},
	Short:   "Manage custom domains",
}

var domainsListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all domains",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		domains, err := client.ListDomains()
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(domains)
		}

		rows := make([][]string, len(domains))
		for i, d := range domains {
			project := "-"
			if d.ProjectID != nil {
				project = (*d.ProjectID)[:8]
			}
			rows[i] = []string{d.ID[:8], d.Domain, project, d.CreatedAt[:10]}
		}
		output.Table([]string{"ID", "DOMAIN", "PROJECT", "CREATED"}, rows)
		return nil
	},
}

var domainsGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get domain details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		domain, err := client.GetDomain(args[0])
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(domain)
		}

		projectID := "-"
		if domain.ProjectID != nil {
			projectID = *domain.ProjectID
		}
		output.Table([]string{"FIELD", "VALUE"}, [][]string{
			{"ID", domain.ID},
			{"Domain", domain.Domain},
			{"Project ID", projectID},
			{"Created", domain.CreatedAt},
		})
		return nil
	},
}

var createDomainName string

var domainsCreateCmd = &cobra.Command{
	Use:   "create <domain>",
	Short: "Register a custom domain",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		domain, err := client.CreateDomain(api.CreateDomainRequest{Domain: args[0]})
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(domain)
		}

		fmt.Printf("✓ Domain registered: %s (ID: %s)\n", domain.Domain, domain.ID)
		return nil
	},
}

var attachProjectID string

var domainsAttachCmd = &cobra.Command{
	Use:   "attach <domain-id> [domain-id...]",
	Short: "Attach one or more domains to a machine",
	Long: `Attach one or more free (unattached) domains to a machine.
All specified domains will point to the same project.

Examples:
  usectl domains attach <domain-id> --project <project-id>
  usectl domains attach <id1> <id2> <id3> --project <project-id>`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}

		for _, domainID := range args {
			domain, err := client.AttachDomain(domainID, api.AttachDomainRequest{ProjectID: attachProjectID})
			if err != nil {
				fmt.Printf("✗ Failed to attach domain %s: %v\n", domainID[:8], err)
				continue
			}

			if jsonOutput {
				output.JSON(domain)
			} else {
				fmt.Printf("✓ Domain %s attached to project %s\n", domain.Domain, attachProjectID[:8])
			}
		}
		return nil
	},
}

var domainsDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a domain",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if err := client.DeleteDomain(args[0]); err != nil {
			return err
		}
		fmt.Println("✓ Domain deleted")
		return nil
	},
}

var (
	attachAppAppID  string
	attachAppDetach bool
)

var domainsAttachAppCmd = &cobra.Command{
	Use:   "attach-app <domain-id>",
	Short: "Pin a domain to a specific app within its project (or detach)",
	Long: `Project-level domains route to whichever single-app project is the default.
Multi-app projects need to pin domains to a specific app — use this to
choose which app a custom domain points at.

Pass --detach to unpin (the domain stays with the machine but goes back
to default routing).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if attachAppDetach {
			if err := client.DetachDomainFromApp(args[0]); err != nil {
				return err
			}
			fmt.Println("✓ Domain detached from app")
			return nil
		}
		if attachAppAppID == "" {
			return fmt.Errorf("--app is required (or pass --detach)")
		}
		if err := client.AttachDomainToApp(args[0], attachAppAppID); err != nil {
			return err
		}
		fmt.Println("✓ Domain attached to app")
		return nil
	},
}

var domainsVerifyCmd = &cobra.Command{
	Use:   "verify <domain-id>",
	Short: "Re-check a domain's CNAME / SSL status (Cloudflare for SaaS)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		resp, err := client.VerifyDomain(args[0])
		if err != nil {
			return err
		}
		return output.JSON(resp)
	},
}

func init() {
	domainsAttachCmd.Flags().StringVar(&attachProjectID, "project", "", "Project ID to attach (required)")
	domainsAttachCmd.MarkFlagRequired("project")

	domainsAttachAppCmd.Flags().StringVar(&attachAppAppID, "app", "", "App UUID to pin the domain to")
	domainsAttachAppCmd.Flags().BoolVar(&attachAppDetach, "detach", false, "Detach the domain from any app (project-level routing)")

	domainsCmd.AddCommand(domainsListCmd)
	domainsCmd.AddCommand(domainsGetCmd)
	domainsCmd.AddCommand(domainsCreateCmd)
	domainsCmd.AddCommand(domainsAttachCmd)
	domainsCmd.AddCommand(domainsAttachAppCmd)
	domainsCmd.AddCommand(domainsVerifyCmd)
	domainsCmd.AddCommand(domainsDeleteCmd)

	rootCmd.AddCommand(domainsCmd)
}
