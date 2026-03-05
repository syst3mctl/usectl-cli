package cmd

import (
	"fmt"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

var s3Cmd = &cobra.Command{
	Use:   "s3",
	Short: "Manage S3 object storage (MinIO) for a project",
	Long: `Manage the S3 bucket provisioned for a project.
Requires the project to have been created with --s3 or toggled on.

Subcommands:
  list      List objects in the bucket (with optional prefix filter)
  download  Download a specific object by key
  toggle    Enable or disable S3 storage for the project`,
}

var s3ListPrefix string

var s3ListCmd = &cobra.Command{
	Use:   "list <project-id>",
	Short: "List objects in a project's S3 bucket",
	Long: `Returns all objects (files and directories) in the project's S3 bucket.
Use --prefix to filter by path prefix.`,
	Example: `  usectl projects s3 list a8f15889
  usectl projects s3 list a8f15889 --prefix uploads/`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}

		objects, err := client.ListS3Objects(args[0], s3ListPrefix)
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(objects)
		}

		if len(objects) == 0 {
			fmt.Println("No objects found.")
			return nil
		}

		rows := make([][]string, len(objects))
		for i, obj := range objects {
			objType := "file"
			size := formatSize(obj.Size)
			modified := obj.LastModified.Format("2006-01-02 15:04")
			if obj.IsDir {
				objType = "dir"
				size = "-"
				modified = "-"
			}
			rows[i] = []string{obj.Key, objType, size, modified}
		}
		output.Table([]string{"KEY", "TYPE", "SIZE", "MODIFIED"}, rows)
		return nil
	},
}

var (
	s3DownloadKey    string
	s3DownloadOutput string
)

var s3DownloadCmd = &cobra.Command{
	Use:   "download <project-id>",
	Short: "Download an object from the project's S3 bucket",
	Long: `Download a single object by its key. The file is saved to the current
directory using the object's filename, or to a custom path with --output.`,
	Example: `  usectl projects s3 download a8f15889 --key uploads/photo.jpg
  usectl projects s3 download a8f15889 --key data.csv --output /tmp/data.csv`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}

		savedPath, err := client.DownloadS3Object(args[0], s3DownloadKey, s3DownloadOutput)
		if err != nil {
			return err
		}

		fmt.Printf("✓ Downloaded %s → %s\n", s3DownloadKey, savedPath)
		return nil
	},
}

var s3ToggleEnable bool

var s3ToggleCmd = &cobra.Command{
	Use:   "toggle <project-id>",
	Short: "Enable or disable S3 storage for a project",
	Long: `Toggle S3 storage on or off. When enabled, a MinIO bucket and dedicated
user are provisioned. When disabled, the S3 flag is cleared (bucket remains).`,
	Example: `  usectl projects s3 toggle a8f15889 --enable
  usectl projects s3 toggle a8f15889 --enable=false`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}

		if err := client.ToggleS3(args[0], s3ToggleEnable); err != nil {
			return err
		}

		action := "disabled"
		if s3ToggleEnable {
			action = "enabled"
		}
		fmt.Printf("✓ S3 storage %s for project %s\n", action, args[0][:8])
		return nil
	},
}

func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func init() {
	// List flags
	s3ListCmd.Flags().StringVar(&s3ListPrefix, "prefix", "", "Filter by key prefix (path/)")

	// Download flags
	s3DownloadCmd.Flags().StringVar(&s3DownloadKey, "key", "", "Object key to download (required)")
	s3DownloadCmd.Flags().StringVar(&s3DownloadOutput, "output", "", "Output file path (default: filename from key)")
	s3DownloadCmd.MarkFlagRequired("key")

	// Toggle flags
	s3ToggleCmd.Flags().BoolVar(&s3ToggleEnable, "enable", false, "Enable S3 (use --enable=false to disable)")

	s3Cmd.AddCommand(s3ListCmd)
	s3Cmd.AddCommand(s3DownloadCmd)
	s3Cmd.AddCommand(s3ToggleCmd)

	// Register under projects
	projectsCmd.AddCommand(s3Cmd)
}
