package cmd

import "github.com/spf13/cobra"

// Machine-scoped command grouping.
//
// Everything that operates on one machine — addons, members, groups, quota,
// variable exposure — used to sit only at the top level, so the command tree
// gave no hint that they all take a machine as their first argument.
// `usectl machines --help` now lists them alongside create/list/pods.
//
// The command objects are SHARED, not duplicated: each stays registered under
// root as well, so `usectl addons list <machine>` keeps working and no
// existing script breaks. Cobra resolves a command by walking down from
// whichever parent the user typed, so both spellings dispatch to the same
// code.
//
// This runs from Execute() rather than from init(). Go runs a package's
// init() functions in filename order, and members.go / quota.go / vars.go
// sort after machine_groups.go — so doing it in init() let their
// rootCmd.AddCommand run last, which reset Parent() back to root and made
// `--help` print "usectl members" instead of "usectl machines members".
// Calling it from Execute() happens after every init() regardless of
// filename, so the grouping cannot be broken by renaming a file.
func attachMachineScopedCommands() {
	// Only commands not already registered under projectsCmd (see
	// projects.go, which already parents s3/cron/envs/domains).
	projectsCmd.AddCommand(
		addonsCmd,  // usectl machines addons <machine>
		membersCmd, // usectl machines members <machine>
		groupsCmd,  // usectl machines groups <machine>
		quotaCmd,   // usectl machines quota <machine>
		varsCmd,    // usectl machines vars <machine>
	)

	// Top-level shortcuts. Once a default machine is set (`usectl use api`),
	// `usectl pods` should work — otherwise `use` only helps inside the
	// `machines ...` prefix, which is most of the typing it was meant to save.
	//
	// Order matters: root FIRST, then projectsCmd, so Parent() ends up as
	// `machines` and help keeps teaching the canonical spelling. Re-adding a
	// command to a parent it already has is a no-op for dispatch and only
	// resets that pointer.
	shortcuts := []*cobra.Command{podsCmd, machineUsageCmd, machineSettingsCmd, enterCmd}
	for _, c := range shortcuts {
		// These are already children of projectsCmd from their own init().
		// AddCommand appends without de-duplicating, so re-adding them listed
		// each one twice in `machines --help`. Remove, add to root, then add
		// back — which also leaves Parent() pointing at `machines`, so help
		// keeps printing the canonical spelling.
		projectsCmd.RemoveCommand(c)
	}
	rootCmd.AddCommand(shortcuts...)
	projectsCmd.AddCommand(shortcuts...)
}
