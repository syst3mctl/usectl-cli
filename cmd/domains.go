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
	Use:   "attach <domain-id>",
	Short: "Attach a domain to a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		domain, err := client.AttachDomain(args[0], api.AttachDomainRequest{ProjectID: attachProjectID})
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(domain)
		}

		fmt.Printf("✓ Domain %s attached to project %s\n", domain.Domain, attachProjectID[:8])
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

func init() {
	domainsAttachCmd.Flags().StringVar(&attachProjectID, "project", "", "Project ID to attach (required)")
	domainsAttachCmd.MarkFlagRequired("project")

	domainsCmd.AddCommand(domainsListCmd)
	domainsCmd.AddCommand(domainsGetCmd)
	domainsCmd.AddCommand(domainsCreateCmd)
	domainsCmd.AddCommand(domainsAttachCmd)
	domainsCmd.AddCommand(domainsDeleteCmd)

	rootCmd.AddCommand(domainsCmd)
}
