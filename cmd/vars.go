package cmd

import (
	"fmt"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

var varsCmd = &cobra.Command{
	Use:   "vars",
	Short: "Manage build-time vs runtime variable exposure (per-app overrides)",
	Long: `Each variable can be exposed as one of three build_target values:

  none       Runtime only (default for secrets) — injected via Secret/EnvFrom.
  build_arg  Passed to Kaniko as --build-arg KEY=VALUE during the image build.
  env_file   Written into a .env file before docker build.

Project-level defaults apply to every variable; per-app columns override
the project default; per-key rows override the per-app default.

See PROMPT_explicit_build_arg_checkbox.md for the full design.`,
}

var (
	varsProjectDefaultBT    string
	varsProjectDotenvAuto   bool
	varsProjectDotenvPath   string
)

var varsProjectDefaultsCmd = &cobra.Command{
	Use:   "project-defaults <project-id>",
	Short: "Set the project-level default build_target + .env file config",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		req := api.ProjectVarDefaultsRequest{
			DefaultBuildTarget: varsProjectDefaultBT,
			BuildDotenvAuto:    varsProjectDotenvAuto,
		}
		if cmd.Flags().Changed("dotenv-path") {
			p := varsProjectDotenvPath
			req.BuildDotenvPath = &p
		}
		if err := client.UpdateProjectVarDefaults(args[0], req); err != nil {
			return err
		}
		fmt.Println("✓ Project var defaults updated")
		return nil
	},
}

var (
	varsAppDefaultBT       string
	varsAppDefaultBTClear  bool
	varsAppDotenvAuto      bool
	varsAppDotenvPath      string
	varsAppDotenvPathClear bool
)

var varsAppDefaultsCmd = &cobra.Command{
	Use:   "app-defaults <project-id> <app-id>",
	Short: "Override the project default for a single app (or clear an override)",
	Long: `Per-app overrides are nullable — pass --clear-build-target or
--clear-dotenv-path to remove an override and inherit the project default.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		req := api.AppVarDefaultsRequest{}
		if varsAppDefaultBTClear {
			empty := ""
			req.DefaultBuildTarget = &empty
		} else if cmd.Flags().Changed("build-target") {
			bt := varsAppDefaultBT
			req.DefaultBuildTarget = &bt
		}
		if cmd.Flags().Changed("dotenv-auto") {
			req.BuildDotenvAuto = &varsAppDotenvAuto
		}
		if varsAppDotenvPathClear {
			empty := ""
			req.BuildDotenvPath = &empty
		} else if cmd.Flags().Changed("dotenv-path") {
			req.BuildDotenvPath = &varsAppDotenvPath
		}
		if err := client.UpdateAppVarDefaults(args[0], args[1], req); err != nil {
			return err
		}
		fmt.Println("✓ App var defaults updated")
		return nil
	},
}

var varsExposureGetCmd = &cobra.Command{
	Use:   "exposure <project-id> <app-id>",
	Short: "Show the resolved exposure (project + app defaults + per-key overrides)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		resp, err := client.GetAppVarExposure(args[0], args[1])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(resp)
		}
		fmt.Printf("Resolved: build_target=%s dotenv_auto=%v",
			resp.Resolved.DefaultBuildTarget, resp.Resolved.BuildDotenvAuto)
		if resp.Resolved.BuildDotenvPath != nil {
			fmt.Printf(" dotenv_path=%s", *resp.Resolved.BuildDotenvPath)
		}
		fmt.Println()
		if len(resp.Overrides) == 0 {
			fmt.Println("No per-key overrides.")
			return nil
		}
		rows := make([][]string, len(resp.Overrides))
		for i, o := range resp.Overrides {
			rt := "no"
			if o.Runtime {
				rt = "yes"
			}
			rows[i] = []string{o.Key, o.BuildTarget, rt}
		}
		output.Table([]string{"KEY", "BUILD_TARGET", "RUNTIME"}, rows)
		return nil
	},
}

var (
	varsExposureBuildTarget string
	varsExposureRuntime     bool
)

var varsExposureSetCmd = &cobra.Command{
	Use:   "expose <project-id> <app-id> <key>",
	Short: "Set a per-variable build-target override for an app",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		exp := api.VarExposure{
			Key:         args[2],
			BuildTarget: varsExposureBuildTarget,
			Runtime:     varsExposureRuntime,
		}
		if err := client.UpsertVarExposure(args[0], args[1], exp); err != nil {
			return err
		}
		fmt.Println("✓ Override saved")
		return nil
	},
}

var varsExposureDeleteCmd = &cobra.Command{
	Use:   "unexpose <project-id> <app-id> <key>",
	Short: "Delete a per-variable override (revert to the resolved default)",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if err := client.DeleteVarExposure(args[0], args[1], args[2]); err != nil {
			return err
		}
		fmt.Println("✓ Override deleted")
		return nil
	},
}

func init() {
	varsProjectDefaultsCmd.Flags().StringVar(&varsProjectDefaultBT, "build-target", "none", "Default build_target: none | build_arg | env_file")
	varsProjectDefaultsCmd.Flags().BoolVar(&varsProjectDotenvAuto, "dotenv-auto", true, "Auto-write .env into container working dir during build")
	varsProjectDefaultsCmd.Flags().StringVar(&varsProjectDotenvPath, "dotenv-path", "", "Explicit relative path to write the build .env (when --dotenv-auto=false)")

	varsAppDefaultsCmd.Flags().StringVar(&varsAppDefaultBT, "build-target", "", "Per-app build_target override")
	varsAppDefaultsCmd.Flags().BoolVar(&varsAppDefaultBTClear, "clear-build-target", false, "Clear the per-app build_target override")
	varsAppDefaultsCmd.Flags().BoolVar(&varsAppDotenvAuto, "dotenv-auto", true, "Auto-write .env at build time")
	varsAppDefaultsCmd.Flags().StringVar(&varsAppDotenvPath, "dotenv-path", "", "Per-app build .env path override")
	varsAppDefaultsCmd.Flags().BoolVar(&varsAppDotenvPathClear, "clear-dotenv-path", false, "Clear the per-app build .env path override")

	varsExposureSetCmd.Flags().StringVar(&varsExposureBuildTarget, "build-target", "build_arg", "Build target: none | build_arg | env_file")
	varsExposureSetCmd.Flags().BoolVar(&varsExposureRuntime, "runtime", true, "Inject as runtime env var too")

	varsCmd.AddCommand(varsProjectDefaultsCmd, varsAppDefaultsCmd,
		varsExposureGetCmd, varsExposureSetCmd, varsExposureDeleteCmd)
	rootCmd.AddCommand(varsCmd)
}
