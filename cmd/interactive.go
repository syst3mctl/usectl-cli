package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// Interactive prompting shared by the `create` wizards.
//
// The rule that keeps this safe for scripts and AI agents: a prompt may only
// appear when stdin is a real terminal AND the caller has not asked for
// unattended operation (--json / --yes). Everywhere else a missing value is a
// hard error naming exactly what is missing, never a hidden blocking read.
// See requireInteractive.

// assumeYes is bound to --yes on the commands that support a wizard.
var assumeYes bool

// interactive reports whether it is safe to prompt.
func interactive() bool {
	if assumeYes || jsonOutput {
		return false
	}
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// requireInteractive returns a machine-readable error when values are missing
// and prompting is not allowed. `missing` names the flags the caller must pass.
func requireInteractive(missing []string, usage string) error {
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("missing required value(s): %s\n  non-interactive mode cannot prompt for these — pass them as flags:\n  %s",
		strings.Join(missing, ", "), usage)
}

var stdinReader = bufio.NewReader(os.Stdin)

// ask prints "label [def]: " and returns the trimmed answer, or def on empty.
func ask(label, def string) string {
	if def != "" {
		fmt.Printf("  %-14s [%s]: ", label, def)
	} else {
		fmt.Printf("  %-14s: ", label)
	}
	line, err := stdinReader.ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return def
	}
	ans := strings.TrimSpace(line)
	if ans == "" {
		return def
	}
	return ans
}

// askFloat re-prompts until the answer parses as a positive number.
func askFloat(label string, def float64) float64 {
	for {
		raw := ask(label, trimFloat(def))
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil || v <= 0 {
			fmt.Printf("     ! %q is not a positive number\n", raw)
			continue
		}
		return v
	}
}

// askSize re-prompts until the answer parses as a size (see parseSizeGB).
func askSize(label string, def float64) float64 {
	for {
		raw := ask(label, trimFloat(def))
		v, err := parseSizeGB(raw)
		if err != nil {
			fmt.Printf("     ! %v\n", err)
			continue
		}
		return v
	}
}

// confirm asks a yes/no question, defaulting to yes.
func confirm(label string) bool {
	if assumeYes {
		return true
	}
	ans := strings.ToLower(ask(label+" [Y/n]", "y"))
	return ans == "" || ans == "y" || ans == "yes"
}

// parseSizeGB reads a size in GB. A bare number is GB; "mb"/"gb"/"g"/"m"
// suffixes are honoured.
//
// The suffix support exists because "--ram 1024" is genuinely ambiguous —
// people mean 1024 MB, but the API field is ram_gb, so taking it literally
// would try to allocate a terabyte. Rather than guess from magnitude (which
// would make "--ram 64" silently mean two different things at different
// scales), the value is always echoed back in GB before anything is created.
func parseSizeGB(s string) (float64, error) {
	raw := strings.ToLower(strings.TrimSpace(s))
	if raw == "" {
		return 0, fmt.Errorf("empty size")
	}
	mult := 1.0
	switch {
	case strings.HasSuffix(raw, "mb"):
		raw, mult = strings.TrimSuffix(raw, "mb"), 1.0/1024
	case strings.HasSuffix(raw, "gb"):
		raw = strings.TrimSuffix(raw, "gb")
	case strings.HasSuffix(raw, "m"):
		raw, mult = strings.TrimSuffix(raw, "m"), 1.0/1024
	case strings.HasSuffix(raw, "g"):
		raw = strings.TrimSuffix(raw, "g")
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a size — use a number of GB, or a suffix like 512mb / 4gb", s)
	}
	if v <= 0 {
		return 0, fmt.Errorf("size must be greater than zero")
	}
	return v * mult, nil
}

// trimFloat renders a float without trailing ".0" noise.
func trimFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
