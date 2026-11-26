package cmd

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

// usectl developer — publish addons to the usectl marketplace
// (developers.usectl.com). Publishing is invite-only; the web portal and
// these commands use the same API.

var (
	devFile       string
	devPublisher  string
	devNotes      string
	devCritical   bool
	devNoRollback bool
	devWait       bool
)

var (
	manifestIDRe   = regexp.MustCompile(`(?m)^id:\s*["']?([a-z0-9-]+)/`)
	manifestTypeRe = regexp.MustCompile(`(?m)^type:\s*["']?([a-z0-9-]+)`)
)

var developerCmd = &cobra.Command{
	Use:     "developer",
	Aliases: []string{"dev"},
	Short:   "Publish addons to the usectl marketplace",
	Long: `Publish addons that tenants add to their machines in one click.

  usectl developer init --publisher acme     write a usectl-addon.yaml to start from
  usectl developer validate                  check it (same rules as a release)
  usectl developer create                    reserve the addon's type and hostname
  usectl developer release --notes "…"       submit a release: checks, then usectl review
  usectl developer status acme-caller        releases, check results, installs

Push your image before releasing; the portal at developers.usectl.com shows
your registry credentials.`,
}

func readManifestFile() (string, error) {
	b, err := os.ReadFile(devFile)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", devFile, err)
	}
	return string(b), nil
}

func manifestPublisher(manifest string) string {
	if devPublisher != "" {
		return devPublisher
	}
	if m := manifestIDRe.FindStringSubmatch(manifest); m != nil {
		return m[1]
	}
	return ""
}

func printCheck(c *api.ManifestCheck) {
	for _, e := range c.Errors {
		fmt.Println("  ✕ " + e)
	}
	for _, w := range c.Warnings {
		fmt.Println("  ! " + w)
	}
	if c.OK {
		fmt.Println("  ✓ manifest is valid")
	}
}

var developerInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Write a usectl-addon.yaml to start from",
	RunE: func(cmd *cobra.Command, args []string) error {
		if devPublisher == "" {
			return errors.New("--publisher is required (your publisher slug, see developers.usectl.com)")
		}
		if _, err := os.Stat(devFile); err == nil {
			return fmt.Errorf("%s already exists", devFile)
		}
		tmpl := strings.ReplaceAll(addonManifestTemplate, "PUBLISHER", devPublisher)
		if err := os.WriteFile(devFile, []byte(tmpl), 0o644); err != nil {
			return err
		}
		fmt.Printf("Wrote %s. Edit it, push your image, then: usectl developer validate\n", devFile)
		return nil
	},
}

var developerValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Check usectl-addon.yaml with the same rules as a release",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		manifest, err := readManifestFile()
		if err != nil {
			return err
		}
		res, err := client.ValidateManifest(manifest, manifestPublisher(manifest))
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(res)
		}
		printCheck(res)
		if !res.OK {
			return errors.New("manifest has errors")
		}
		return nil
	},
}

var developerCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create the addon from usectl-addon.yaml (reserves its type and hostname)",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		manifest, err := readManifestFile()
		if err != nil {
			return err
		}
		pub := manifestPublisher(manifest)
		if pub == "" {
			return errors.New("could not tell the publisher from id:; pass --publisher")
		}
		if err := client.CreatePublisherAddon(pub, manifest); err != nil {
			return err
		}
		fmt.Println("Created. Push your image, then: usectl developer release --notes \"…\"")
		return nil
	},
}

var developerReleaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Submit a release: automatic checks, then usectl review",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		manifest, err := readManifestFile()
		if err != nil {
			return err
		}
		m := manifestTypeRe.FindStringSubmatch(manifest)
		if m == nil {
			return errors.New("no type: in the manifest")
		}
		addonType := m[1]
		rel, err := client.SubmitAddonRelease(addonType, api.SubmitReleaseRequest{
			Manifest: manifest, Notes: devNotes, Critical: devCritical, DowngradeUnsafe: devNoRollback,
		})
		if err != nil {
			return err
		}
		fmt.Printf("Submitted %s %s — checks are running.\n", addonType, rel.Version)
		if !devWait {
			fmt.Printf("Follow along: usectl developer status %s\n", addonType)
			return nil
		}
		for {
			time.Sleep(10 * time.Second)
			a, err := client.GetDeveloperAddon(addonType)
			if err != nil {
				return err
			}
			for _, r := range a.Releases {
				if r.ID != rel.ID || r.Status == "checking" {
					continue
				}
				printRelease(r)
				if r.Status == "rejected" {
					return errors.New("the release did not pass")
				}
				return nil
			}
		}
	},
}

func printRelease(r api.DeveloperRelease) {
	fmt.Printf("%s  %s\n", r.Version, r.Status)
	for _, g := range r.Gates {
		line := fmt.Sprintf("  %-8s %-8s", g.Name, g.Status)
		if g.Detail != "" {
			line += " " + g.Detail
		}
		fmt.Println(line)
	}
	if r.ReviewNote != "" {
		fmt.Println("  review: " + r.ReviewNote)
	}
}

var developerStatusCmd = &cobra.Command{
	Use:   "status <addon-type>",
	Short: "Show an addon's releases, check results and installs",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		a, err := client.GetDeveloperAddon(args[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(a)
		}
		fmt.Printf("%s (%s) — %s, host <machine>-%s.usectl.com\n\n", a.Listing.Name, a.Listing.AddonType, a.Listing.Channel, a.Listing.Slug)
		if len(a.Releases) == 0 {
			fmt.Println("No releases yet.")
		}
		for _, r := range a.Releases {
			printRelease(r)
		}
		if len(a.Installs) > 0 {
			fmt.Println("\nInstalls:")
			for v, states := range a.Installs {
				parts := []string{}
				for st, n := range states {
					parts = append(parts, fmt.Sprintf("%d %s", n, st))
				}
				fmt.Printf("  %s: %s\n", v, strings.Join(parts, ", "))
			}
		}
		return nil
	},
}

func init() {
	developerCmd.PersistentFlags().StringVarP(&devFile, "file", "f", "usectl-addon.yaml", "manifest file")
	developerCmd.PersistentFlags().StringVar(&devPublisher, "publisher", "", "publisher slug (default: from the manifest id)")
	developerReleaseCmd.Flags().StringVar(&devNotes, "notes", "", "release notes tenants read before updating")
	developerReleaseCmd.Flags().BoolVar(&devCritical, "critical", false, "security fix: usectl applies it everywhere 3 days after release")
	developerReleaseCmd.Flags().BoolVar(&devNoRollback, "no-rollback", false, "this version changes data irreversibly; tenants cannot roll back from it")
	developerReleaseCmd.Flags().BoolVar(&devWait, "wait", false, "wait for the automatic checks to finish")
	developerCmd.AddCommand(developerInitCmd, developerValidateCmd, developerCreateCmd, developerReleaseCmd, developerStatusCmd)
	rootCmd.AddCommand(developerCmd)
}

const addonManifestTemplate = `# usectl-addon.yaml
schema: 1
id: PUBLISHER/my-addon           # never changes
type: PUBLISHER-my-addon         # never changes
slug: myaddon                    # hostnames: <machine>-myaddon.usectl.com
name: My Addon
description: One sentence tenants see in the add-pod menu.
category: tools
icon: "🧩"
version: 1.0.0

image:
  repository: registry.systemctl.sh/addons-PUBLISHER/my-addon
  tag: 1.0.0                     # resolved to a digest when you release

runtime:
  port: 8080                     # >= 1024, the container runs as non-root
  uid: 10001
  mount_path: /data              # the persistent disk
  grace_period_seconds: 30
  health:
    path: /healthz
    startup: {period_seconds: 10, failure_threshold: 30}

presets:
  - {name: small,  cpu_millis: 500,  memory_mib: 1024, storage_gib: 5}
  - {name: medium, cpu_millis: 1000, memory_mib: 2048, storage_gib: 10}

secrets:
  - {env: ADMIN_PASSWORD, kind: password, length: 24, config_key: PASS}

env:
  PORT: "{{port}}"
  PUBLIC_URL: "{{public_url}}"

outputs:
  - {key: URL, label: URL, value: "{{public_url}}", kind: url}
  - {key: PASS, label: Admin password, secret: ADMIN_PASSWORD, kind: secret}

ingress:
  public: true

panel:
  tabs:
    - builtin: overview
    - builtin: settings
    - builtin: logs

# Tabs you fill yourself (usectl calls your addon, server-to-server, with a
# 60 s handoff token in Authorization; see the guide):
#   - {title: Calls, type: table, source: /usectl/v1/calls,
#      columns: [{key: caller, label: Caller}, {key: status, label: Status, kind: badge}],
#      row_actions: [replay]}
#   - {title: Dashboard, type: view, source: /usectl/v1/views/dashboard, refresh_seconds: 30}
#   - {title: Maintenance, type: actions, items: [restore]}
#
# actions:
#   - {id: replay, label: Replay, path: /usectl/v1/actions/replay}
#   - {id: restore, label: Restore backup, path: /usectl/v1/actions/restore, role: owner,
#      confirm: "Replace the current data?", fields: [{name: key, label: Backup key, required: true}]}

egress: []                       # hosts your addon calls, shown to tenants
`
