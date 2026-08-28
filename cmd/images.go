package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

// `usectl images push` — ship a locally built image into the platform registry
// without ever holding a registry credential.
//
// The upload goes straight from here to object storage via a presigned URL;
// the platform then pushes it into Harbor from inside the cluster. That split
// is why this is three calls rather than one: image tarballs run 200MB–1GB+,
// and routing them through the API server would starve the log streams and
// terminals it also serves.

var (
	imagePushTag  string
	imagePushFile string
	imagePushKeep bool
)

var imagesCmd = &cobra.Command{
	Use:   "images",
	Short: "Ship prebuilt container images to your machines",
}

var imagesPushCmd = &cobra.Command{
	Use:   "push <project-id> <app-id> <local-image>",
	Short: "Upload a locally built image and deploy it",
	Long: `Push an image you built locally into the platform registry.

Runs 'docker save' on the image, uploads the tarball directly to object
storage, and asks the platform to move it into the registry. You never handle
registry credentials — the push happens inside the cluster.

The app must be image-sourced (usectl apps create --image, or
usectl apps update --image).`,
	Example: `  # Build locally, then ship it
  docker build -t myapi:v3 .
  usectl images push a8f15889 web-app-id myapi:v3

  # Tag it explicitly in the platform registry
  usectl images push a8f15889 web-app-id myapi:v3 --tag v3.0.1

  # Skip docker save and upload a tar you already have
  usectl images push a8f15889 web-app-id --file ./image.tar`,
	Args: cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		projectID, appID := args[0], args[1]

		tarPath := imagePushFile
		cleanup := false

		if tarPath == "" {
			if len(args) < 3 {
				return fmt.Errorf("provide a local image name, or --file with an existing tar")
			}
			localImage := args[2]
			// docker save writes to a temp file rather than a pipe: the
			// presigned PUT is signed for an exact content length, so the
			// size has to be known before the upload starts.
			tmp, terr := os.CreateTemp("", "usectl-image-*.tar")
			if terr != nil {
				return fmt.Errorf("create temp file: %w", terr)
			}
			tarPath = tmp.Name()
			_ = tmp.Close()
			cleanup = !imagePushKeep

			fmt.Printf("Saving %s…\n", localImage)
			save := exec.Command("docker", "save", "-o", tarPath, localImage)
			save.Stderr = os.Stderr
			if serr := save.Run(); serr != nil {
				os.Remove(tarPath)
				return fmt.Errorf("docker save failed — is %q built locally? (%w)", localImage, serr)
			}
			if imagePushTag == "" {
				// Reuse the local tag when the user didn't name one, so the
				// registry mirrors what they built.
				if i := strings.LastIndex(localImage, ":"); i != -1 && !strings.Contains(localImage[i:], "/") {
					imagePushTag = localImage[i+1:]
				}
			}
		}
		if cleanup {
			defer os.Remove(tarPath)
		}

		st, err := os.Stat(tarPath)
		if err != nil {
			return fmt.Errorf("read image tar: %w", err)
		}

		fmt.Println("Requesting upload URL…")
		ticket, err := client.StartImageUpload(projectID, appID)
		if err != nil {
			return err
		}
		if ticket.MaxBytes > 0 && st.Size() > ticket.MaxBytes {
			return fmt.Errorf("image is %s; the limit is %s",
				humanBytes(st.Size()), humanBytes(ticket.MaxBytes))
		}

		fmt.Printf("Uploading %s…\n", humanBytes(st.Size()))
		lastPct := -1
		err = api.UploadImageTar(ticket.UploadURL, tarPath, func(sent, total int64) {
			if total == 0 {
				return
			}
			pct := int(sent * 100 / total)
			// Only redraw on a whole-percent change, and only on a TTY —
			// piping this into a CI log should not produce 10,000 lines.
			if pct != lastPct && isTTY() {
				lastPct = pct
				fmt.Printf("\r  %d%% (%s / %s)", pct, humanBytes(sent), humanBytes(total))
			}
		})
		if isTTY() {
			fmt.Println()
		}
		if err != nil {
			return err
		}

		fmt.Println("Pushing into the registry…")
		res, err := client.CompleteImageUpload(projectID, appID, ticket.UploadKey, imagePushTag)
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(res)
		}
		fmt.Printf("✓ %s\n", res.Message)
		fmt.Printf("  Image: %s\n", res.ImageRef)
		fmt.Printf("  Job:   %s\n", res.Job)
		fmt.Printf("\n  Deploy it:\n    usectl projects deploy %s\n", projectID)
		return nil
	},
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGT"[exp])
}

func init() {
	imagesPushCmd.Flags().StringVar(&imagePushTag, "tag", "", "Tag to use in the platform registry (default: the local tag, else a timestamp)")
	imagesPushCmd.Flags().StringVar(&imagePushFile, "file", "", "Upload an existing tar instead of running docker save")
	imagesPushCmd.Flags().BoolVar(&imagePushKeep, "keep-tar", false, "Keep the temporary tar produced by docker save")
	imagesCmd.AddCommand(imagesPushCmd)
	rootCmd.AddCommand(imagesCmd)
}

// isTTY reports whether stdout is a terminal. Progress is redrawn with \r,
// which turns a CI log into thousands of lines when it is not.
func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
